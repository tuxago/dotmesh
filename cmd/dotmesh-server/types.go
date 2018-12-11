package main

import (
	"io"
	"sync"

	"github.com/dotmesh-io/dotmesh/pkg/container"
	"github.com/dotmesh-io/dotmesh/pkg/observer"
	"github.com/dotmesh-io/dotmesh/pkg/registry"

	"github.com/coreos/etcd/client"

	dmclient "github.com/dotmesh-io/dotmesh/pkg/client"
	"github.com/dotmesh-io/dotmesh/pkg/types"
	"github.com/dotmesh-io/dotmesh/pkg/user"
)

const DEFAULT_BRANCH = "master"

type dirtyInfo struct {
	Server     string
	DirtyBytes int64
	SizeBytes  int64
}

type PermissionDenied struct {
}

func (e PermissionDenied) Error() string {
	return "Permission denied."
}

// Aliases
type User = user.User
type SafeUser = user.SafeUser
type CloneWithName = types.CloneWithName
type ClonesList = types.ClonesList
type TopLevelFilesystem = types.TopLevelFilesystem
type VolumesAndBranches = types.VolumesAndBranches
type Server = types.Server
type Origin = types.Origin
type PathToTopLevelFilesystem = types.PathToTopLevelFilesystem
type DotmeshVolume = types.DotmeshVolume
type VolumeName = types.VolumeName
type RegistryFilesystem = types.RegistryFilesystem

type ByAddress []Server

type dotmeshVolumeByName []DotmeshVolume

func (v dotmeshVolumeByName) Len() int {
	return len(v)
}

func (v dotmeshVolumeByName) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}

func (v dotmeshVolumeByName) Less(i, j int) bool {
	return v[i].Name.Name < v[j].Name.Name
}

type DotmeshVolumeAndContainers struct {
	Volume     DotmeshVolume
	Containers []container.DockerContainer
}

type VersionInfo struct {
	InstalledVersion    string `json:"installed_version"`
	CurrentVersion      string `json:"current_version"`
	CurrentReleaseDate  int    `json:"current_release_date"`
	CurrentDownloadURL  string `json:"current_download_url"`
	CurrentChangelogURL string `json:"current_changelog_url"`
	ProjectWebsite      string `json:"project_website"`
	Outdated            bool   `json:"outdated"`
}

// type fsMap map[string]*fsMachine

type transferFn func(
	f *fsMachine,
	fromFilesystemId, fromSnapshotId, toFilesystemId, toSnapshotId string,
	transferRequestId string,
	client *dmclient.JsonRpcClient, transferRequest *types.TransferRequest,
) (*Event, stateFn)

// Defaults are specified in main.go

type SafeConfig struct {
}

// state machinery
type stateFn func(*fsMachine) stateFn

type Snapshot = types.Snapshot
type Clone = types.Clone
type Filesystem = types.Filesystem
type Metadata = types.Metadata

type TransferUpdateKind int

const (
	TransferStart TransferUpdateKind = iota
	TransferGotIds
	TransferCalculatedSize
	TransferTotalAndSize
	TransferProgress
	TransferIncrementIndex
	TransferNextS3File
	TransferSent
	TransferFinished
	TransferStatus

	TransferGetCurrentPollResult
)

type TransferUpdate struct {
	Kind TransferUpdateKind

	Changes TransferPollResult

	GetResult chan TransferPollResult
}

type StateManager interface {
	InitFilesystemMachine(filesystemId string) (*fsMachine, error)
	GetFilesystemMachine(filesystemId string) (*fsMachine, error)

	// ActivateFilesystem(filesystemId string) error
	AlignMountStateWithMasters(filesystemId string) error
	ActivateClone(topLevelFilesystemId, originFilesystemId, originSnapshotId, newCloneFilesystemId, newBranchName string) (string, error)
	DeleteFilesystem(filesystemId string) error
	DeleteFilesystemFromMap(filesystemId string)
	// current node ID
	NodeID() string

	UpdateSnapshotsFromKnownState(server, filesystem string, snapshots []*Snapshot) error
	SnapshotsFor(server string, filesystemId string) ([]Snapshot, error)
	SnapshotsForCurrentMaster(filesystemId string) ([]Snapshot, error)

	AddressesForServer(server string) []string

	// TODO: move under a separate interface for Etcd related things
	MarkFilesystemAsLiveInEtcd(topLevelFilesystemId string) error
}

// a "filesystem machine" or "filesystem state machine"
type fsMachine struct {
	// which ZFS filesystem this statemachine is operating on
	filesystemId string
	filesystem   *Filesystem

	// channels for uploading and downloading file data
	fileInputIO  chan *InputFile
	fileOutputIO chan *OutputFile

	// channel of requests going in to the state machine
	requests chan *Event
	// inner versions of the above
	innerRequests chan *Event
	// inner responses don't need to be parameterized on request id because
	// they're guaranteed to only have one goroutine reading on the channel.
	innerResponses chan *Event
	// channel of responses coming out of the state machine, indexed by request
	// id so that multiple goroutines reading responses for the same filesystem
	// id won't get the wrong result.
	responses     map[string]chan *Event
	responsesLock *sync.Mutex
	// channel notifying etcd-updater whenever snapshot state changes
	snapshotsModified chan bool
	// pointer to global state, because it's convenient to have access to it
	// state *InMemoryState

	containerClient container.Client
	etcdClient      client.KeysAPI
	state           StateManager
	userManager     user.UserManager
	registry        registry.Registry

	localReceiveProgress observer.Observer
	newSnapsOnMaster     observer.Observer

	// fsMachines live forever, whereas filesystem structs do not. so
	// filesystem struct's snapshotLock can live here so that it doesn't get
	// clobbered
	snapshotsLock *sync.Mutex
	// a place to store arguments to pass to the next state
	handoffRequest *Event
	// filesystem-sliced view of new snapshot events
	newSnapsOnServers observer.Observer
	// current state, status field for reporting/debugging and transition observer
	currentState            string
	status                  string
	lastTransitionTimestamp int64
	transitionObserver      observer.Observer
	lastS3TransferRequest   types.S3TransferRequest
	lastTransferRequest     types.TransferRequest
	lastTransferRequestId   string
	pushCompleted           chan bool
	dirtyDelta              int64
	sizeBytes               int64
	transferUpdates         chan TransferUpdate
	// only to be accessed via the updateEtcdAboutTransfers goroutine!
	currentPollResult TransferPollResult

	// state machine metadata
	// Moved from InMemoryState:
	// server id => filesystem id => state machine metadata
	//globalStateCache:     make(map[string]map[string]map[string]string),

	// new structure: NodeID => State machine metadata
	stateMachineMetadata   map[string]map[string]string
	stateMachineMetadataMu *sync.RWMutex

	// NodeID => snapshot metadata
	snapshotCache   map[string][]*Snapshot
	snapshotCacheMu *sync.RWMutex

	filesystemMetadataTimeout int64
}

// EventArgs - alias to make refactoring easier,
// TODO: use the types directly and remove alias
type EventArgs = types.EventArgs
type Event = types.Event

// type EventArgs map[string]interface{}
// type Event struct {
// 	Name string
// 	Args *EventArgs
// }

// InputFile is used to write files to the disk on the local node.
type InputFile struct {
	Filename string
	Contents io.Reader
	User     string
	Response chan *Event
}

// OutputFile is used to read files from the disk on the local node
// this is always done against a specific, already mounted snapshotId
// the mount path of the snapshot is passed through via SnapshotMountPath
type OutputFile struct {
	Filename          string
	SnapshotMountPath string
	Contents          io.Writer
	User              string
	Response          chan *Event
}

// func (ea EventArgs) String() string {
// 	aggr := []string{}
// 	for k, v := range ea {
// 		aggr = append(aggr, fmt.Sprintf("%s: %+q", k, v))
// 	}
// 	return strings.Join(aggr, ", ")
// }

// func (e Event) String() string {
// 	return fmt.Sprintf("<Event %s: %s>", e.Name, e.Args)
// }

type TransferPollResult struct {
	TransferRequestId string
	Peer              string // hostname
	User              string
	ApiKey            string
	Direction         string // "push" or "pull"

	// Hold onto this information, it might become useful for e.g. recursive
	// receives of clone filesystems.
	LocalNamespace   string
	LocalName        string
	LocalBranchName  string
	RemoteNamespace  string
	RemoteName       string
	RemoteBranchName string

	// Same across both clusters
	FilesystemId string

	// TODO add clusterIds? probably comes from etcd. in fact, could be the
	// discovery id (although that is only for bootstrap... hmmm).
	InitiatorNodeId string
	PeerNodeId      string

	// XXX a Transfer that spans multiple filesystem ids won't have a unique
	// starting/target snapshot, so this is in the wrong place right now.
	// although maybe it makes sense to talk about a target *final* snapshot,
	// with interim snapshots being an implementation detail.
	StartingCommit string
	TargetCommit   string

	Index              int    // i.e. transfer 1/4 (Index=1)
	Total              int    //                   (Total=4)
	Status             string // one of "starting", "running", "finished", "error"
	NanosecondsElapsed int64
	Size               int64 // size of current segment in bytes
	Sent               int64 // number of bytes of current segment sent so far
	Message            string
}

type Config struct {
	FilesystemMetadataTimeout int64
	UserManager               user.UserManager
	EtcdClient                client.KeysAPI

	// variables used to create fsm.FsMachine
	ZFSExecPath string
	ZPoolPath   string
	PoolName    string
}

// refers to a clone's "pointer" to a filesystem id and its snapshot.
//
// note that a clone's Origin's FilesystemId may differ from the "top level"
// filesystemId in the Registry's Clones map if the clone is attributed to a
// top-level filesystem which is *transitively* its parent but not its direct
// parent. In this case the Origin FilesystemId will always point to its direct
// parent.

func castToMetadata(val interface{}) Metadata {
	meta, ok := val.(Metadata)
	if !ok {
		meta = Metadata{}
		// massage the data into the right type
		cast := val.(map[string]interface{})
		for k, v := range cast {
			meta[k] = v.(string)
		}
	}
	return meta
}

type Prelude struct {
	SnapshotProperties []*Snapshot
}

type containerInfo struct {
	Server     string
	Containers []container.DockerContainer
}
