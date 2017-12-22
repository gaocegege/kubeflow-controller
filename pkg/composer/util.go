package composer

import (
	"fmt"

	"k8s.io/kubernetes/pkg/api/v1"
)

// GenerateRuntimeID generates runtimeID.
func GenerateRuntimeID() string {
	return v1.SimpleNameGenerator.GenerateName("")
}

func GeneratePodName(name, typ, runtimeID string) string {
	return fmt.Sprintf("%s-%s-%s", name, typ, runtimeID)
}
