// package api

// import (
// 	"encoding/binary"
// 	"errors"
// 	"io"

// 	"github.com/flashbots/mev-boost-relay/services/api/hashes"
// )

// const (
// 	TreeDepth = 32
// )

// type Node struct {
// 	Hash  hashes.H256
// 	Left  *MerkleTree
// 	Right *MerkleTree
// }

// type MerkleTree struct {
// 	Leaf hashes.H256
// 	Node
// 	Zero int
// }

// type Proof struct {
// 	Leaf  hashes.H256
// 	Index int
// 	Path  [TreeDepth]hashes.H256
// }

// func merkleRootFromBranch(leaf hashes.H256, branch []hashes.H256, depth, index int) hashes.H256 {
// 	current := leaf

// 	for i, next := range branch {
// 		ithBit := (index >> (depth - 1 - i)) & 0x01
// 		if ithBit == 1 {
// 			current = hashes.HashConcat(next, current)
// 		} else {
// 			current = hashes.HashConcat(current, next)
// 		}
// 	}

// 	return current
// }

// func verifyMerkleProof(leaf hashes.H256, branch []hashes.H256, depth, index int, root hashes.H256) bool {
// 	if len(branch) == depth {
// 		return merkleRootFromBranch(leaf, branch, depth, index) == root
// 	}
// 	return false
// }

// func (p *Proof) Root() hashes.H256 {
// 	return merkleRootFromBranch(p.Leaf, p.Path[:], TreeDepth, p.Index)
// }

// func (p *Proof) Encode(w io.Writer) error {
// 	if _, err := w.Write(p.Leaf[:]); err != nil {
// 		return err
// 	}
// 	if err := binary.Write(w, binary.BigEndian, uint64(p.Index)); err != nil {
// 		return err
// 	}
// 	for _, hash := range p.Path {
// 		if _, err := w.Write(hash[:]); err != nil {
// 			return err
// 		}
// 	}
// 	return nil
// }

// func (p *Proof) Decode(r io.Reader) error {
// 	if _, err := io.ReadFull(r, p.Leaf[:]); err != nil {
// 		return err
// 	}
// 	if err := binary.Read(r, binary.BigEndian, &p.Index); err != nil {
// 		return err
// 	}
// 	for i := range p.Path {
// 		if _, err := io.ReadFull(r, p.Path[i][:]); err != nil {
// 			return err
// 		}
// 	}
// 	return nil
// }

// func (tree *MerkleTree) Hash() hashes.H256 {
// 	switch {
// 	case tree.Leaf != (hashes.H256{}):
// 		return tree.Leaf
// 	case tree.Zero != 0:
// 		return hashes.ZERO_HASHES[tree.Zero]
// 	default:
// 		return tree.Node.Hash
// 	}
// }

// func (tree *MerkleTree) create(leaves []hashes.H256, depth int) *MerkleTree {
// 	switch {
// 	case len(leaves) == 0:
// 		return &MerkleTree{Zero: depth}
// 	case depth == 0:
// 		return &MerkleTree{Leaf: leaves[0]}
// 	default:
// 		subtreeCapacity := 1 << (uint(depth) - 1)
// 		leftLeaves, rightLeaves := leaves, []hashes.H256{}
// 		if len(leaves) > subtreeCapacity {
// 			leftLeaves, rightLeaves = leaves[:subtreeCapacity], leaves[subtreeCapacity:]
// 		}

// 		leftSubtree := tree.create(leftLeaves, depth-1)
// 		rightSubtree := tree.create(rightLeaves, depth-1)
// 		hash := hashes.HashConcat(leftSubtree.Hash(), rightSubtree.Hash())

// 		n := Node{Hash: hash, Left: leftSubtree, Right: rightSubtree}

// 		return &MerkleTree{Node: n}
// 	}
// }

// func (tree *MerkleTree) Create(leaves []hashes.H256, depth int) *MerkleTree {
// 	return tree.create(leaves, depth)
// }

// func (tree *MerkleTree) PushLeaf(elem hashes.H256, depth int) error {
// 	if depth == 0 {
// 		return errors.New("DepthTooSmall")
// 	}

// 	switch {
// 	case tree.Leaf != (hashes.H256{}):
// 		return errors.New("LeafReached")
// 	case tree.Zero != 0:
// 		*tree = *tree.Create([]hashes.H256{elem}, depth)
// 	default:
// 		left := tree.Left
// 		right := tree.Right
// 		switch {
// 		case left.Leaf != (hashes.H256{}):
// 			if right.Leaf != (hashes.H256{}) {
// 				return errors.New("MerkleTreeFull")
// 			}
// 			*left = *tree.Create([]hashes.H256{elem}, depth-1)
// 		case right.Leaf != (hashes.H256{}):
// 			*right = *tree.Create([]hashes.H256{elem}, depth-1)
// 		case left.Zero != 0 && right.Zero != 0:
// 			*left = *tree.Create([]hashes.H256{elem}, depth-1)
// 		case left.Leaf != (hashes.H256{}):
// 			if err := left.PushLeaf(elem, depth-1); err != nil {
// 				if errors.Is(err, errors.New("MerkleTreeFull")) {
// 					*right = *tree.Create([]hashes.H256{elem}, depth-1)
// 				} else {
// 					return err
// 				}
// 			}
// 		case right.Zero != 0:
// 			*right = *tree.Create([]hashes.H256{elem}, depth-1)
// 		default:
// 			return errors.New("Invalid")
// 		}

// 		tree.Node.Hash = hashes.HashConcat(left.Hash(), right.Hash())
// 	}

// 	return nil
// }

// func (tree *MerkleTree) LeftAndRightBranches() (*MerkleTree, *MerkleTree) {
// 	switch {
// 	case tree.Leaf != (hashes.H256{}), tree.Zero == 0:
// 		return nil, nil
// 	case tree.Left != nil && tree.Right != nil:
// 		return tree.Left, tree.Right
// 	default:
// 		return &zeroNodes[tree.Zero-1], &zeroNodes[tree.Zero-1]
// 	}
// }

// func (tree *MerkleTree) IsLeaf() bool {
// 	return tree.Leaf != (hashes.H256{})
// }

// // GenerateProof returns the leaf at `index` and a Merkle proof of its inclusion.
// // The Merkle proof is in "bottom-up" order, starting with a leaf node and moving up the tree.
// // Its length will be exactly equal to `depth`.
// func (node *Node) GenerateProof(index, depth int) (hashes.H256, []hashes.H256) {
// 	var proof []hashes.H256
// 	currentNode := node
// 	currentDepth := depth

// 	for currentDepth > 0 {
// 		ithBit := (index >> (currentDepth - 1)) & 0x01
// 		left, right := currentNode.LeftAndRightBranches()

// 		// Go right, include the left branch in the proof.
// 		if ithBit == 1 {
// 			proof = append(proof, left.Hash())
// 			currentNode = right
// 		} else {
// 			proof = append(proof, right.Hash())
// 			currentNode = left
// 		}
// 		currentDepth--
// 	}

// 	// Ensure that the proof length matches the expected depth.
// 	if len(proof) != depth {
// 		panic("Proof length does not match the expected depth")
// 	}

// 	// Put proof in bottom-up order.
// 	for i, j := 0, len(proof)-1; i < j; i, j = i+1, j-1 {
// 		proof[i], proof[j] = proof[j], proof[i]
// 	}

// 	return currentNode.Hash(), proof
// }
