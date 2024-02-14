package sparse

import (
	"bytes"
	"crypto/sha256"
	"errors"

	"github.com/flashbots/mev-boost-relay/services/api/hashes"
	"github.com/flashbots/mev-boost-relay/services/api/merkle"
)

var ZERO_HASHES []hashes.H256

// TODO fix this
func init() {
	ZERO_HASHES = make([]hashes.H256, hashes.TREE_DEPTH)
	for i := range ZERO_HASHES {
		ZERO_HASHES[i] = sha256.Sum256(nil)
	}
}

type Proof struct {
	Leaf  hashes.H256
	Index uint
	Path  []hashes.H256
}

func (p *Proof) AsLatest() *Proof {
	modifiedPath := make([]hashes.H256, hashes.TREE_DEPTH)
	for i := 0; i < hashes.TREE_DEPTH; i++ {
		size := p.Index >> uint(i)
		if size&1 == 1 {
			modifiedPath[i] = p.Path[i]
		} else {
			modifiedPath[i] = ZERO_HASHES[i]
		}
	}

	return &Proof{
		Leaf:  p.Leaf,
		Index: p.Index,
		Path:  modifiedPath,
	}
}

type MerkleTree interface {
	ProveAgainstCurrent(index uint64) Proof

	ProveAgainstPrevious(leaf_index uint32, root_index uint32) (Proof, error)
}

type SparseMerkleTree struct {
	Tree merkle.MerkleTreeNode
}

// TODO maybe change array 32 to be of type [32]hashes.H256
func (smt SparseMerkleTree) ProveAgainstCurrent(index uint64) Proof {
	root, proof := merkle.GenerateProof(smt.Tree, index, hashes.TREE_DEPTH)

	var array32 []hashes.H256
	copy(array32[:], proof[:32])

	return Proof{
		Leaf:  root,
		Index: uint(index),
		Path:  array32,
	}
}

func (smt SparseMerkleTree) ProveAgainstPrevious(leaf_index uint32, root_index uint32) (Proof, error) {

	if root_index <= leaf_index {
		return Proof{}, errors.New("Root less than leaf")
	}

	root_proof := smt.ProveAgainstCurrent(uint64(root_index))

	root := root_proof.AsLatest()

	leaf_proof := smt.ProveAgainstCurrent(uint64(leaf_index))

	leaf_tree := From(&leaf_proof)

	tree := From(root).merge(*leaf_tree)

	return tree.ProveAgainstCurrent(uint64(leaf_index)), nil
}

// TODO fix this
func (smt SparseMerkleTree) hash() hashes.H256 {
	switch t := smt.Tree.(type) {
	case *merkle.LeafNode:
		return t.GetRoot()
	case *merkle.InnerNode:
		return t.GetRoot()
	case *merkle.ZeroNode:
		return t.GetRoot()
	default:
		return hashes.H256{}
	}
}

// / Merges the sparse merkle tree `b` into `self` via DFS.
// /
// / A node in `self` is merged with a node in `b` iff the hashes of both
// / nodes are equal.
func (smt SparseMerkleTree) merge(b SparseMerkleTree) SparseMerkleTree {
	switch t := smt.Tree.(type) {
	case *merkle.ZeroNode:
		return smt
	case *merkle.LeafNode:
		temp := t.GetRoot()
		temptwo := b.hash()
		if bytes.Equal(temp[:], temptwo[:]) {
			return b
		} else {
			return smt
		}
	case *merkle.InnerNode:
		switch bt := b.Tree.(type) {
		case *merkle.LeafNode:
			return smt
		case *merkle.ZeroNode:
			return smt
		case *merkle.InnerNode:

			a_left := t.Left()

			left_a_tree := SparseMerkleTree{Tree: a_left}

			left_b_tree := SparseMerkleTree{Tree: bt.Left()}

			merged_left := left_a_tree.merge(left_b_tree)

			a_right := t.Right()

			right_a_tree := SparseMerkleTree{Tree: a_right}

			right_b_tree := SparseMerkleTree{Tree: bt.Right()}

			merged_right := right_a_tree.merge(right_b_tree)

			mergedHash := hashes.HashConcat(merged_left.hash(), merged_right.hash())

			t_root := t.GetRoot()
			if !bytes.Equal(mergedHash[:], t_root[:]) {
				panic("merged hash not equal to current node hash")
			}

			inner := merkle.CreateInner(merged_left.Tree, merged_right.Tree)
			return SparseMerkleTree{Tree: inner}
		default:
			return smt
		}
	default:
		return smt
	}
}

func From(value *Proof) *SparseMerkleTree {

	var leaves []hashes.H256

	tree := merkle.Create(leaves, 0)

	//TODO I need to create a merkle tree with the below

	tree = &merkle.LeafNode{Hash: value.Leaf}

	for i := 0; i < hashes.TREE_DEPTH; i++ {
		index := value.Index >> uint(i)
		if index&1 == 1 {
			left := &merkle.LeafNode{Hash: value.Path[i]}
			tree = merkle.CreateInner(left, tree)
		} else {
			right := &merkle.LeafNode{Hash: value.Path[i]}
			tree = merkle.CreateInner(tree, right)
		}
	}
	return &SparseMerkleTree{tree}
}
