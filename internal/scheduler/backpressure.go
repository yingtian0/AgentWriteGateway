package scheduler

type BackpressureReason string

const (
	BackpressureRunnerCapacity BackpressureReason = "RUNNER_CAPACITY_EXHAUSTED"
	BackpressureAdapterRate    BackpressureReason = "ADAPTER_RATE_LIMITED"
	BackpressureQueueDepth     BackpressureReason = "QUEUE_DEPTH_EXCEEDED"
)

type Capacity struct {
	RunnerAvailable  int `json:"runner_available"`
	AdapterRemaining int `json:"adapter_remaining"`
	QueueDepth       int `json:"queue_depth"`
	QueueLimit       int `json:"queue_limit"`
}

func (capacity Capacity) Available() (int, BackpressureReason) {
	if capacity.RunnerAvailable <= 0 {
		return 0, BackpressureRunnerCapacity
	}
	if capacity.AdapterRemaining <= 0 {
		return 0, BackpressureAdapterRate
	}
	if capacity.QueueLimit <= 0 || capacity.QueueDepth >= capacity.QueueLimit {
		return 0, BackpressureQueueDepth
	}
	available := min(capacity.RunnerAvailable, capacity.AdapterRemaining, capacity.QueueLimit-capacity.QueueDepth)
	return available, ""
}
