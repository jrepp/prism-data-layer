module github.com/prism/prism-operator

go 1.21

require (
	github.com/kedacore/keda/v2 v2.12.0
	k8s.io/api v0.28.3
	k8s.io/apimachinery v0.28.3
	k8s.io/client-go v0.28.3
	sigs.k8s.io/controller-runtime v0.16.3
)

require (
	github.com/go-logr/logr v1.2.4
	github.com/onsi/ginkgo/v2 v2.12.0
	github.com/onsi/gomega v1.27.10
)
