package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_library",
)

go_library(
    name = "go_default_library",
    srcs = [
        "autoscaling_client.go",
        "doc.go",
        "generated_expansion.go",
        "horizontalpodautoscaler.go",
    ],
    importmap = "github.com/dotmesh-io/dotmesh/vendor/k8s.io/client-go/kubernetes/typed/autoscaling/v1",
    importpath = "k8s.io/client-go/kubernetes/typed/autoscaling/v1",
    tags = ["automanaged"],
    deps = [
        "//vendor/k8s.io/apimachinery/pkg/apis/meta/v1:go_default_library",
        "//vendor/k8s.io/apimachinery/pkg/runtime/serializer:go_default_library",
        "//vendor/k8s.io/apimachinery/pkg/types:go_default_library",
        "//vendor/k8s.io/apimachinery/pkg/watch:go_default_library",
        "//vendor/k8s.io/client-go/kubernetes/scheme:go_default_library",
        "//vendor/k8s.io/client-go/pkg/apis/autoscaling/v1:go_default_library",
        "//vendor/k8s.io/client-go/rest:go_default_library",
    ],
)
