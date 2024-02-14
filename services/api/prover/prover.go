package prover

import (
	"encoding/hex"
	"fmt"

	"github.com/flashbots/mev-boost-relay/services/api/hashes"
	"github.com/flashbots/mev-boost-relay/services/api/merkle"
	"github.com/flashbots/mev-boost-relay/services/api/sparse"
)

// Prover is a depth-32 sparse Merkle tree capable of producing proofs for arbitrary elements.
type Prover struct {
	count int
	tree  sparse.SparseMerkleTree
}

// ProverError represents errors related to the Prover.
type ProverError struct {
	Message string
}

func (e *ProverError) Error() string {
	return e.Message
}

// NewProverError creates a new ProverError with the given message.
func NewProverError(message string) *ProverError {
	return &ProverError{Message: message}
}

// NewProver creates a new Prover with an empty tree.
func NewProver() *Prover {
	var leaves []hashes.H256
	fullTree := merkle.Create(leaves, hashes.TREE_DEPTH)

	smt := sparse.SparseMerkleTree{Tree: fullTree}

	return &Prover{
		count: 0,
		tree:  smt,
	}
}

// Ingest adds a leaf to the tree and returns the new root hash.
func (p *Prover) Ingest(element hashes.H256) (hashes.H256, error) {
	p.count++
	var err error

	p.tree.Tree, err = p.tree.Tree.PushLeaf(element, hashes.TREE_DEPTH)
	if err != nil {
		return hashes.H256{}, NewProverError(fmt.Sprintf("Failed to push leaf to tree: %v", err))
	}
	return p.tree.Tree.GetRoot(), nil
}

// Root returns the current root hash of the tree.
func (p *Prover) Root() hashes.H256 {
	return p.tree.Tree.GetRoot()
}

// Count returns the number of leaves that have been ingested.
func (p *Prover) Count() int {
	return p.count
}

// ProveAgainstPrevious creates a proof of a leaf in this tree.
func (p *Prover) ProveAgainstPrevious(leafIndex, rootIndex int) (*sparse.Proof, error) {
	if rootIndex > int(^uint32(0)) {
		return nil, NewProverError("Requested proof for index above u32::MAX")
	}
	count := p.Count()
	if rootIndex >= count {
		return nil, NewProverError(fmt.Sprintf("Requested proof for a zero element. Requested: %d. Tree has: %d", rootIndex, count))
	}
	proof, err := p.tree.ProveAgainstPrevious(uint32(leafIndex), uint32(rootIndex))
	if err != nil {
		return nil, NewProverError(fmt.Sprintf("Failed to create proof: %v", err))
	}
	return &proof, nil
}

// Verify verifies a proof against this tree's root.
func (p *Prover) Verify(proof *sparse.Proof) error {

	actual := merkle.MerkleRootFromBranch(proof.Leaf, proof.Path, hashes.TREE_DEPTH, int(proof.Index))

	expected := p.Root()
	if expected != actual {
		return NewProverError(fmt.Sprintf("Proof verification failed. Root is %s, produced is %s", hex.EncodeToString(expected[:]), hex.EncodeToString(actual[:])))
	}
	return nil
}

func main() {
	// Your test code can go here
}
