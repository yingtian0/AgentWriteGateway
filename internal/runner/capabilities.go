package runner

import "agentwritegateway/pkg/protocol"

type CapabilitySet map[protocol.Capability]struct{}

func NewCapabilitySet(values ...protocol.Capability) CapabilitySet {
	result := make(CapabilitySet, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func (s CapabilitySet) Allows(capability protocol.Capability) bool {
	_, ok := s[capability]
	return ok
}
