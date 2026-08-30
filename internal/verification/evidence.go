package verification

import (
	"errors"

	"agentwritegateway/pkg/adapter"
)

func ValidateEvidence(evidence adapter.Evidence) error {
	if err := adapter.ValidateEvidence(evidence); err != nil {
		return err
	}
	hash, err := adapter.EvidenceHash(evidence)
	if err != nil {
		return err
	}
	if hash != evidence.EvidenceHash {
		return errors.New("evidence hash mismatch")
	}
	return nil
}
