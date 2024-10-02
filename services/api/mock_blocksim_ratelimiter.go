package api

import (
	"context"

	"github.com/AnomalyFi/baton/common"
)

type MockBlockSimulationRateLimiter struct {
}

func (m *MockBlockSimulationRateLimiter) SimBlockAndGetGasUsed(context context.Context, blockReq *common.BlockValidationRequest) (uint64, error, error) {
	return 0, nil, nil
}

func (m *MockBlockSimulationRateLimiter) CurrentCounter() int64 {
	return 0
}
