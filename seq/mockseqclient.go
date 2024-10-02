// Package seq contains logic to interface with seq types
package seq

import (
	"context"

	"github.com/AnomalyFi/hypersdk/chain"
	hrpc "github.com/AnomalyFi/hypersdk/rpc"
	"github.com/ava-labs/avalanchego/ids"
)

type MockSeqClient struct {
	onNewBlockHandler func(*chain.StatefulBlock, *hrpc.NextProposerReply)
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

func (s *MockSeqClient) TriggerOnNextBlock(blk *chain.StatefulBlock, nextProposer *hrpc.NextProposerReply) {
	s.onNewBlockHandler(blk, nextProposer)
}

func (s *MockSeqClient) SetOnNewBlockHandler(handler func(*chain.StatefulBlock, *hrpc.NextProposerReply)) {
	s.onNewBlockHandler = handler
}
