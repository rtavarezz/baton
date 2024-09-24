package seq

import (
	"context"
	"github.com/AnomalyFi/hypersdk/chain"
	hrpc "github.com/AnomalyFi/hypersdk/rpc"
)

type MockSeqClient struct {
	headSlot       uint64
	newSlotHandler func(uint64)
}

func (cli *MockSeqClient) SetNewSlotHandler(handler func(uint64)) {
	cli.newSlotHandler = handler
}

func (cli *MockSeqClient) TriggerNextSlot(slot uint64) {
	cli.newSlotHandler(slot)
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

func (s *MockSeqClient) nextProposer(ctx context.Context) (*hrpc.NextProposerReply, error) {
	return nil, nil
}
