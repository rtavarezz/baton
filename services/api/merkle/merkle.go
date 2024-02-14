package merkle

import (
	"errors"
	"hash"
	"sync"

	"github.com/flashbots/mev-boost-relay/services/api/hashes"
	"github.com/flashbots/mev-boost-relay/services/api/slice"
	"github.com/flashbots/mev-boost-relay/services/api/trie"

	"github.com/minio/sha256-simd"
)

const (
	// DepositContractDepth is the maximum tree depth as defined by EIP-4881.
	DepositContractDepth = 32
)

var (
	// ErrFinalizedNodeCannotPushLeaf may occur when attempting to push a leaf to a finalized node. When a node is finalized, it cannot be modified or changed.
	ErrFinalizedNodeCannotPushLeaf = errors.New("can't push a leaf to a finalized node")
	// ErrLeafNodeCannotPushLeaf may occur when attempting to push a leaf to a leaf node.
	ErrLeafNodeCannotPushLeaf = errors.New("can't push a leaf to a leaf node")
	// ErrZeroLevel occurs when the value of level is 0.
	ErrZeroLevel = errors.New("level should be greater than 0")
	// ErrZeroDepth occurs when the value of depth is 0.
	ErrZeroDepth = errors.New("depth should be greater than 0")
)

// MerkleTreeNode is the interface for a Merkle tree.
type MerkleTreeNode interface {
	// GetRoot returns the root of the Merkle tree.
	GetRoot() hashes.H256
	// IsFull returns whether there is space left for deposits.
	IsFull() bool
	// Finalize marks deposits of the Merkle tree as finalized.
	//Finalize(depositsToFinalize uint64, depth uint64) (MerkleTreeNode, error)
	// GetFinalized returns the number of deposits and a list of hashes of all the finalized nodes.
	//GetFinalized(result []hashes.H256) (uint64, []hashes.H256)
	// PushLeaf adds a new leaf node at the next available Zero node.
	PushLeaf(leaf hashes.H256, depth uint64) (MerkleTreeNode, error)

	// Right represents the right child of a node.
	Right() MerkleTreeNode
	// Left represents the left child of a node.
	Left() MerkleTreeNode
}

// create builds a new merkle tree
func Create(leaves []hashes.H256, depth uint64) MerkleTreeNode {
	length := uint64(len(leaves))
	if length == 0 {
		return &ZeroNode{depth: depth}
	}
	if depth == 0 {
		return &LeafNode{Hash: leaves[0]}
	}
	split := Min(PowerOf2(depth-1), length)
	left := Create(leaves[0:split], depth-1)
	right := Create(leaves[split:], depth-1)
	return &InnerNode{left: left, right: right}
}

// CreateInner makes a new inner node
func CreateInner(left MerkleTreeNode, right MerkleTreeNode) MerkleTreeNode {
	return &InnerNode{left: left, right: right}
}

// GenerateProof returns a merkle proof and root
func GenerateProof(tree MerkleTreeNode, index uint64, depth uint64) (hashes.H256, []hashes.H256) {
	var proof []hashes.H256
	node := tree
	for depth > 0 {
		ithBit := (index >> (depth - 1)) & 0x1
		if ithBit == 1 {
			proof = append(proof, node.Left().GetRoot())
			node = node.Right()
		} else {
			proof = append(proof, node.Right().GetRoot())
			node = node.Left()
		}
		depth--
	}
	proof = slice.Reverse(proof)
	return node.GetRoot(), proof
}

// VerifyMerkleProof verifies a proof that `leaf` exists at `index` in a Merkle tree rooted at `root`.
// The `branch` argument is the main component of the proof: it should be a list of internal node hashes
// such that the root can be reconstructed (in bottom-up order).
func VerifyMerkleProof(leaf hashes.H256, branch []hashes.H256, depth, index int, root hashes.H256) bool {
	if len(branch) == depth {
		return MerkleRootFromBranch(leaf, branch, depth, index) == root
	}
	return false
}

// MerkleRootFromBranch computes a root hash from a leaf and a Merkle proof.
func MerkleRootFromBranch(leaf hashes.H256, branch []hashes.H256, depth, index int) hashes.H256 {
	if len(branch) != depth {
		panic("Proof length does not match the expected depth")
	}

	current := leaf

	for i, next := range branch {
		ithBit := (index >> i) & 0x01
		if ithBit == 1 {
			current = hashes.HashConcat(next, current)
		} else {
			current = hashes.HashConcat(current, next)
		}
	}

	return current
}

// // FinalizedNode represents a finalized node and satisfies the MerkleTreeNode interface.
// type FinalizedNode struct {
// 	depositCount uint64
// 	hash         hashes.H256
// }

// // GetRoot returns the root of the Merkle tree.
// func (f *FinalizedNode) GetRoot() hashes.H256 {
// 	return f.hash
// }

// // IsFull returns whether there is space left for deposits.
// // A FinalizedNode will always return true as by definition it
// // is full and deposits can't be added to it.
// func (_ *FinalizedNode) IsFull() bool {
// 	return true
// }

// // Finalize marks deposits of the Merkle tree as finalized.
// func (f *FinalizedNode) Finalize(depositsToFinalize uint64, depth uint64) (MerkleTreeNode, error) {
// 	return f, nil
// }

// // GetFinalized returns a list of hashes of all the finalized nodes and the number of deposits.
// func (f *FinalizedNode) GetFinalized(result []hashes.H256) (uint64, []hashes.H256) {
// 	return f.depositCount, append(result, f.hash)
// }

// // PushLeaf adds a new leaf node at the next available zero node.
// func (_ *FinalizedNode) PushLeaf(_ hashes.H256, _ uint64) (MerkleTreeNode, error) {
// 	return nil, ErrFinalizedNodeCannotPushLeaf
// }

// // Right returns nil as a finalized node can't have any children.
// func (_ *FinalizedNode) Right() MerkleTreeNode {
// 	return nil
// }

// // Left returns nil as a finalized node can't have any children.
// func (_ *FinalizedNode) Left() MerkleTreeNode {
// 	return nil
// }

// LeafNode represents a leaf node holding a deposit and satisfies the MerkleTreeNode interface.
type LeafNode struct {
	Hash hashes.H256
}

// GetRoot returns the root of the Merkle tree.
func (l *LeafNode) GetRoot() hashes.H256 {
	return l.Hash
}

// IsFull returns whether there is space left for deposits.
// A LeafNode will always return true as it is the last node
// in the tree and therefore can't have any deposits added to it.
func (_ *LeafNode) IsFull() bool {
	return true
}

// // Finalize marks deposits of the Merkle tree as finalized.
// func (l *LeafNode) Finalize(depositsToFinalize uint64, depth uint64) (MerkleTreeNode, error) {
// 	return &FinalizedNode{1, l.Hash}, nil
// }

// GetFinalized returns a list of hashes of all the finalized nodes and the number of deposits.
// func (_ *LeafNode) GetFinalized(result []hashes.H256) (uint64, []hashes.H256) {
// 	return 0, result
// }

// PushLeaf adds a new leaf node at the next available zero node.
func (_ *LeafNode) PushLeaf(_ hashes.H256, _ uint64) (MerkleTreeNode, error) {
	return nil, ErrLeafNodeCannotPushLeaf
}

// Right returns nil as a leaf node is the last node and can't have any children.
func (_ *LeafNode) Right() MerkleTreeNode {
	return nil
}

// Left returns nil as a leaf node is the last node and can't have any children.
func (_ *LeafNode) Left() MerkleTreeNode {
	return nil
}

// InnerNode represents an inner node with two children and satisfies the MerkleTreeNode interface.
type InnerNode struct {
	left, right MerkleTreeNode
}

// GetRoot returns the root of the Merkle tree.
func (n *InnerNode) GetRoot() hashes.H256 {
	left := n.left.GetRoot()
	right := n.right.GetRoot()
	return Hash(append(left[:], right[:]...))
}

// IsFull returns whether there is space left for deposits.
func (n *InnerNode) IsFull() bool {
	return n.right.IsFull()
}

// // Finalize marks deposits of the Merkle tree as finalized.
// func (n *InnerNode) Finalize(depositsToFinalize uint64, depth uint64) (MerkleTreeNode, error) {
// 	var err error
// 	deposits := PowerOf2(depth)
// 	if deposits <= depositsToFinalize {
// 		return &FinalizedNode{deposits, n.GetRoot()}, nil
// 	}
// 	if depth == 0 {
// 		return &ZeroNode{}, ErrZeroDepth
// 	}
// 	n.left, err = n.left.Finalize(depositsToFinalize, depth-1)
// 	if err != nil {
// 		return &ZeroNode{}, err
// 	}
// 	if depositsToFinalize > deposits/2 {
// 		remaining := depositsToFinalize - deposits/2
// 		n.right, err = n.right.Finalize(remaining, depth-1)
// 		if err != nil {
// 			return &ZeroNode{}, err
// 		}
// 	}
// 	return n, nil
// }

// GetFinalized returns a list of hashes of all the finalized nodes and the number of deposits.
// func (n *InnerNode) GetFinalized(result []hashes.H256) (uint64, []hashes.H256) {
// 	leftDeposits, result := n.left.GetFinalized(result)
// 	rightDeposits, result := n.right.GetFinalized(result)
// 	return leftDeposits + rightDeposits, result
// }

// PushLeaf adds a new leaf node at the next available zero node.
func (n *InnerNode) PushLeaf(leaf hashes.H256, depth uint64) (MerkleTreeNode, error) {
	if !n.left.IsFull() {
		left, err := n.left.PushLeaf(leaf, depth-1)
		if err == nil {
			n.left = left
		} else {
			return n, err
		}
	} else {
		right, err := n.right.PushLeaf(leaf, depth-1)
		if err == nil {
			n.right = right
		} else {
			return n, err
		}
	}
	return n, nil
}

// Right returns the child node on the right.
func (n *InnerNode) Right() MerkleTreeNode {
	return n.right
}

// Left returns the child node on the left.
func (n *InnerNode) Left() MerkleTreeNode {
	return n.left
}

// ZeroNode represents an empty node without a deposit and satisfies the MerkleTreeNode interface.
type ZeroNode struct {
	depth uint64
}

// GetRoot returns the root of the Merkle tree.
func (z *ZeroNode) GetRoot() hashes.H256 {
	if z.depth == DepositContractDepth {
		return Hash(append(trie.ZeroHashes[z.depth-1][:], trie.ZeroHashes[z.depth-1][:]...))
	}
	return trie.ZeroHashes[z.depth]
}

// IsFull returns wh   ether there is space left for deposits.
// A ZeroNode will always return false as a ZeroNode is an empty node
// that gets replaced by a deposit.
func (_ *ZeroNode) IsFull() bool {
	return false
}

// Finalize marks deposits of the Merkle tree as finalized.
func (_ *ZeroNode) Finalize(depositsToFinalize uint64, depth uint64) (MerkleTreeNode, error) {
	return &ZeroNode{}, nil
}

// // GetFinalized returns a list of hashes of all the finalized nodes and the number of deposits.
// func (_ *ZeroNode) GetFinalized(result []hashes.H256) (uint64, []hashes.H256) {
// 	return 0, result
// }

// PushLeaf adds a new leaf node at the next available zero node.
func (_ *ZeroNode) PushLeaf(leaf hashes.H256, depth uint64) (MerkleTreeNode, error) {
	return Create([]hashes.H256{leaf}, depth), nil
}

// Right returns nil as a zero node can't have any children.
func (_ *ZeroNode) Right() MerkleTreeNode {
	return nil
}

// Left returns nil as a zero node can't have any children.
func (_ *ZeroNode) Left() MerkleTreeNode {
	return nil
}

func PowerOf2(n uint64) uint64 {
	if n >= 64 {
		panic("integer overflow")
	}
	return 1 << n
}

// Min returns the smaller integer of the two
// given ones. This is used over the Min function
// in the standard math library because that min function
// has to check for some special floating point cases
// making it slower by a magnitude of 10.
func Min(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

var sha256Pool = sync.Pool{New: func() interface{} {
	return sha256.New()
}}

// Hash defines a function that returns the sha256 checksum of the data passed in.
// https://github.com/ethereum/consensus-specs/blob/v0.9.3/specs/core/0_beacon-chain.md#hash
func Hash(data []byte) [32]byte {
	h, ok := sha256Pool.Get().(hash.Hash)
	if !ok {
		h = sha256.New()
	}
	defer sha256Pool.Put(h)
	h.Reset()

	var b [32]byte

	// The hash interface never returns an error, for that reason
	// we are not handling the error below. For reference, it is
	// stated here https://golang.org/pkg/hash/#Hash

	// #nosec G104
	h.Write(data)
	h.Sum(b[:0])

	return b
}
