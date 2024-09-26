package seq

import (
	"context"

	"github.com/AnomalyFi/hypersdk/chain"
	hrpc "github.com/AnomalyFi/hypersdk/rpc"
	"github.com/ava-labs/avalanchego/ids"
)

type MockSeqClient struct {
	headSlot       uint64
	newSlotHandler func(uint64)
}

func (s *MockSeqClient) SetNewSlotHandler(handler func(uint64)) {
	s.newSlotHandler = handler
}

func (s *MockSeqClient) TriggerNextSlot(slot uint64) {
	s.newSlotHandler(slot)
}

func NewMockSeqClient(_ *SeqClientConfig) (*MockSeqClient, error) {
	return &MockSeqClient{}, nil
}

func (s *MockSeqClient) SeqHead() *chain.StatefulBlock {
	return nil
}

func (s *MockSeqClient) NamespaceExists() bool {
	return true
}

func (s *MockSeqClient) SetNamespace(namespace []byte) {
}

func (s *MockSeqClient) NextProposer(ctx context.Context) *hrpc.NextProposerReply {
	return nil
}

func (s *MockSeqClient) GetChainID() ids.ID {
	return ids.Empty
}

func (s *MockSeqClient) GetNetworkID() uint32 {
	return 1337
}

func (s *MockSeqClient) CurrentValidators(ctx context.Context) []*hrpc.Validator {
	return nil
}
