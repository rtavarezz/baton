package incremental

import (
	"github.com/flashbots/mev-boost-relay/services/api/hashes"
)

type IncrementalMerkle struct {
	Branch [hashes.TREE_DEPTH]hashes.H256
	Count  uint64
}

func NewIncrementalMerkle() IncrementalMerkle {
	var branch [hashes.TREE_DEPTH]hashes.H256
	for i := range branch {
		branch[i] = hashes.ZERO_HASHES[i]
	}
	return IncrementalMerkle{
		Branch: branch,
		Count:  0,
	}
}

func (im *IncrementalMerkle) Ingest(element hashes.H256) {
	var node hashes.H256 = element
	if im.Count >= (1 << 32) {
		panic("count exceeds maximum value")
	}
	im.Count++
	size := im.Count
	for i := 0; i < hashes.TREE_DEPTH; i++ {
		if (size & 1) == 1 {
			im.Branch[i] = node
			return
		}
		node = hashes.HashConcat(im.Branch[i], node)
		size /= 2
	}
}

func (im *IncrementalMerkle) Root() hashes.H256 {
	var node hashes.H256
	size := im.Count

	for i, elem := range im.Branch {
		if (size & 1) == 1 {
			node = hashes.HashConcat(elem, node)
		} else {
			node = hashes.HashConcat(node, hashes.ZERO_HASHES[i])
		}
		size /= 2
	}

	return node
}

func (im *IncrementalMerkle) count() uint64 {
	return im.Count
}

func (im *IncrementalMerkle) Index() uint32 {
	if im.Count == 0 {
		panic("index is invalid when tree is empty")
	}
	return uint32(im.Count - 1)
}

func (im *IncrementalMerkle) branch() *[hashes.TREE_DEPTH]hashes.H256 {
	return &im.Branch
}

func branchRoot(item hashes.H256, branch [hashes.TREE_DEPTH]hashes.H256, index int) hashes.H256 {
	return merkleRootFromBranch(item, branch, 32, index)
}

func (im *IncrementalMerkle) verify(proof *Proof) bool {
	computed := branchRoot(proof.Leaf, proof.Path, proof.Index)
	return computed == im.Root()
}

func merkleRootFromBranch(item hashes.H256, branch [hashes.TREE_DEPTH]hashes.H256, size int, index int) hashes.H256 {
	// Implement your merkle root calculation logic here
	return item
}

type Proof struct {
	Leaf  hashes.H256
	Path  [hashes.TREE_DEPTH]hashes.H256
	Index int
}

type MerkleTreeInsertion struct {
	Leaf_index uint32
	Message_id hashes.H256
}

type Checkpoint struct {
	/// The merkle tree hook address
	Merkle_tree_hook_address hashes.H256
	/// The mailbox / merkle tree hook domain
	Mailbox_domain uint32
	/// The checkpointed root
	Root hashes.H256
	/// The index of the checkpoint
	Index uint32
}

type CheckpointWithMessageId struct {
	/// existing Hyperlane checkpoint struct
	Checkpoint Checkpoint
	/// hash of message emitted from mailbox checkpoint.index
	Message_id hashes.H256
}
