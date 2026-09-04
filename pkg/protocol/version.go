package protocol

import "fmt"

const VersionV1Alpha1 = "themisy.protocol/v1alpha1"

func ValidateVersion(version string) error {
	if version != VersionV1Alpha1 {
		return fmt.Errorf("UNSUPPORTED_PROTOCOL_VERSION: %q", version)
	}
	return nil
}
