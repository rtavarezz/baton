package api

import (
	"context"

	"github.com/flashbots/mev-boost-relay/common"
)

type MockBlockSimulationRateLimiter struct {
	simulationError    error
	tobSimulationError error
}

func (m *MockBlockSimulationRateLimiter) Send(context context.Context, payload *common.BuilderBlockValidationRequest, isHighPrio, fastTrack bool) (error, error) {
	return nil, m.simulationError
}

func (m *MockBlockSimulationRateLimiter) SimBlockAndGetGasUsed(context context.Context, blockReq *common.BlockValidationRequest) (uint64, error, error) {
	return 0, nil, nil
}

func (m *MockBlockSimulationRateLimiter) CurrentCounter() int64 {
	return 0
}
