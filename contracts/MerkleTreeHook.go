// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package contracts

import (
	"errors"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
	_ = abi.ConvertType
)

// MerkleLibTree is an auto generated low-level Go binding around an user-defined struct.
type MerkleLibTree struct {
	Branch [32][32]byte
	Count  *big.Int
}

// MerkleTreeHookMetaData contains all meta data concerning the MerkleTreeHook contract.
var MerkleTreeHookMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_mailbox\",\"type\":\"address\"}],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"bytes32\",\"name\":\"messageId\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"uint32\",\"name\":\"index\",\"type\":\"uint32\"}],\"name\":\"InsertedIntoTree\",\"type\":\"event\"},{\"inputs\":[],\"name\":\"count\",\"outputs\":[{\"internalType\":\"uint32\",\"name\":\"\",\"type\":\"uint32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"deployedBlock\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"latestCheckpoint\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"},{\"internalType\":\"uint32\",\"name\":\"\",\"type\":\"uint32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes\",\"name\":\"\",\"type\":\"bytes\"},{\"internalType\":\"bytes\",\"name\":\"message\",\"type\":\"bytes\"}],\"name\":\"postDispatch\",\"outputs\":[],\"stateMutability\":\"payable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes\",\"name\":\"\",\"type\":\"bytes\"},{\"internalType\":\"bytes\",\"name\":\"\",\"type\":\"bytes\"}],\"name\":\"quoteDispatch\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"pure\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"root\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"tree\",\"outputs\":[{\"components\":[{\"internalType\":\"bytes32[32]\",\"name\":\"branch\",\"type\":\"bytes32[32]\"},{\"internalType\":\"uint256\",\"name\":\"count\",\"type\":\"uint256\"}],\"internalType\":\"structMerkleLib.Tree\",\"name\":\"\",\"type\":\"tuple\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
}

// MerkleTreeHookABI is the input ABI used to generate the binding from.
// Deprecated: Use MerkleTreeHookMetaData.ABI instead.
var MerkleTreeHookABI = MerkleTreeHookMetaData.ABI

// MerkleTreeHook is an auto generated Go binding around an Ethereum contract.
type MerkleTreeHook struct {
	MerkleTreeHookCaller     // Read-only binding to the contract
	MerkleTreeHookTransactor // Write-only binding to the contract
	MerkleTreeHookFilterer   // Log filterer for contract events
}

// MerkleTreeHookCaller is an auto generated read-only Go binding around an Ethereum contract.
type MerkleTreeHookCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// MerkleTreeHookTransactor is an auto generated write-only Go binding around an Ethereum contract.
type MerkleTreeHookTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// MerkleTreeHookFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type MerkleTreeHookFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// MerkleTreeHookSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type MerkleTreeHookSession struct {
	Contract     *MerkleTreeHook   // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// MerkleTreeHookCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type MerkleTreeHookCallerSession struct {
	Contract *MerkleTreeHookCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts         // Call options to use throughout this session
}

// MerkleTreeHookTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type MerkleTreeHookTransactorSession struct {
	Contract     *MerkleTreeHookTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts         // Transaction auth options to use throughout this session
}

// MerkleTreeHookRaw is an auto generated low-level Go binding around an Ethereum contract.
type MerkleTreeHookRaw struct {
	Contract *MerkleTreeHook // Generic contract binding to access the raw methods on
}

// MerkleTreeHookCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type MerkleTreeHookCallerRaw struct {
	Contract *MerkleTreeHookCaller // Generic read-only contract binding to access the raw methods on
}

// MerkleTreeHookTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type MerkleTreeHookTransactorRaw struct {
	Contract *MerkleTreeHookTransactor // Generic write-only contract binding to access the raw methods on
}

// NewMerkleTreeHook creates a new instance of MerkleTreeHook, bound to a specific deployed contract.
func NewMerkleTreeHook(address common.Address, backend bind.ContractBackend) (*MerkleTreeHook, error) {
	contract, err := bindMerkleTreeHook(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &MerkleTreeHook{MerkleTreeHookCaller: MerkleTreeHookCaller{contract: contract}, MerkleTreeHookTransactor: MerkleTreeHookTransactor{contract: contract}, MerkleTreeHookFilterer: MerkleTreeHookFilterer{contract: contract}}, nil
}

// NewMerkleTreeHookCaller creates a new read-only instance of MerkleTreeHook, bound to a specific deployed contract.
func NewMerkleTreeHookCaller(address common.Address, caller bind.ContractCaller) (*MerkleTreeHookCaller, error) {
	contract, err := bindMerkleTreeHook(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &MerkleTreeHookCaller{contract: contract}, nil
}

// NewMerkleTreeHookTransactor creates a new write-only instance of MerkleTreeHook, bound to a specific deployed contract.
func NewMerkleTreeHookTransactor(address common.Address, transactor bind.ContractTransactor) (*MerkleTreeHookTransactor, error) {
	contract, err := bindMerkleTreeHook(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &MerkleTreeHookTransactor{contract: contract}, nil
}

// NewMerkleTreeHookFilterer creates a new log filterer instance of MerkleTreeHook, bound to a specific deployed contract.
func NewMerkleTreeHookFilterer(address common.Address, filterer bind.ContractFilterer) (*MerkleTreeHookFilterer, error) {
	contract, err := bindMerkleTreeHook(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &MerkleTreeHookFilterer{contract: contract}, nil
}

// bindMerkleTreeHook binds a generic wrapper to an already deployed contract.
func bindMerkleTreeHook(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := MerkleTreeHookMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_MerkleTreeHook *MerkleTreeHookRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _MerkleTreeHook.Contract.MerkleTreeHookCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_MerkleTreeHook *MerkleTreeHookRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _MerkleTreeHook.Contract.MerkleTreeHookTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_MerkleTreeHook *MerkleTreeHookRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _MerkleTreeHook.Contract.MerkleTreeHookTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_MerkleTreeHook *MerkleTreeHookCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _MerkleTreeHook.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_MerkleTreeHook *MerkleTreeHookTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _MerkleTreeHook.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_MerkleTreeHook *MerkleTreeHookTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _MerkleTreeHook.Contract.contract.Transact(opts, method, params...)
}

// Count is a free data retrieval call binding the contract method 0x06661abd.
//
// Solidity: function count() view returns(uint32)
func (_MerkleTreeHook *MerkleTreeHookCaller) Count(opts *bind.CallOpts) (uint32, error) {
	var out []interface{}
	err := _MerkleTreeHook.contract.Call(opts, &out, "count")

	if err != nil {
		return *new(uint32), err
	}

	out0 := *abi.ConvertType(out[0], new(uint32)).(*uint32)

	return out0, err

}

// Count is a free data retrieval call binding the contract method 0x06661abd.
//
// Solidity: function count() view returns(uint32)
func (_MerkleTreeHook *MerkleTreeHookSession) Count() (uint32, error) {
	return _MerkleTreeHook.Contract.Count(&_MerkleTreeHook.CallOpts)
}

// Count is a free data retrieval call binding the contract method 0x06661abd.
//
// Solidity: function count() view returns(uint32)
func (_MerkleTreeHook *MerkleTreeHookCallerSession) Count() (uint32, error) {
	return _MerkleTreeHook.Contract.Count(&_MerkleTreeHook.CallOpts)
}

// DeployedBlock is a free data retrieval call binding the contract method 0x82ea7bfe.
//
// Solidity: function deployedBlock() view returns(uint256)
func (_MerkleTreeHook *MerkleTreeHookCaller) DeployedBlock(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _MerkleTreeHook.contract.Call(opts, &out, "deployedBlock")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// DeployedBlock is a free data retrieval call binding the contract method 0x82ea7bfe.
//
// Solidity: function deployedBlock() view returns(uint256)
func (_MerkleTreeHook *MerkleTreeHookSession) DeployedBlock() (*big.Int, error) {
	return _MerkleTreeHook.Contract.DeployedBlock(&_MerkleTreeHook.CallOpts)
}

// DeployedBlock is a free data retrieval call binding the contract method 0x82ea7bfe.
//
// Solidity: function deployedBlock() view returns(uint256)
func (_MerkleTreeHook *MerkleTreeHookCallerSession) DeployedBlock() (*big.Int, error) {
	return _MerkleTreeHook.Contract.DeployedBlock(&_MerkleTreeHook.CallOpts)
}

// LatestCheckpoint is a free data retrieval call binding the contract method 0x907c0f92.
//
// Solidity: function latestCheckpoint() view returns(bytes32, uint32)
func (_MerkleTreeHook *MerkleTreeHookCaller) LatestCheckpoint(opts *bind.CallOpts) ([32]byte, uint32, error) {
	var out []interface{}
	err := _MerkleTreeHook.contract.Call(opts, &out, "latestCheckpoint")

	if err != nil {
		return *new([32]byte), *new(uint32), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)
	out1 := *abi.ConvertType(out[1], new(uint32)).(*uint32)

	return out0, out1, err

}

// LatestCheckpoint is a free data retrieval call binding the contract method 0x907c0f92.
//
// Solidity: function latestCheckpoint() view returns(bytes32, uint32)
func (_MerkleTreeHook *MerkleTreeHookSession) LatestCheckpoint() ([32]byte, uint32, error) {
	return _MerkleTreeHook.Contract.LatestCheckpoint(&_MerkleTreeHook.CallOpts)
}

// LatestCheckpoint is a free data retrieval call binding the contract method 0x907c0f92.
//
// Solidity: function latestCheckpoint() view returns(bytes32, uint32)
func (_MerkleTreeHook *MerkleTreeHookCallerSession) LatestCheckpoint() ([32]byte, uint32, error) {
	return _MerkleTreeHook.Contract.LatestCheckpoint(&_MerkleTreeHook.CallOpts)
}

// QuoteDispatch is a free data retrieval call binding the contract method 0xaaccd230.
//
// Solidity: function quoteDispatch(bytes , bytes ) pure returns(uint256)
func (_MerkleTreeHook *MerkleTreeHookCaller) QuoteDispatch(opts *bind.CallOpts, arg0 []byte, arg1 []byte) (*big.Int, error) {
	var out []interface{}
	err := _MerkleTreeHook.contract.Call(opts, &out, "quoteDispatch", arg0, arg1)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// QuoteDispatch is a free data retrieval call binding the contract method 0xaaccd230.
//
// Solidity: function quoteDispatch(bytes , bytes ) pure returns(uint256)
func (_MerkleTreeHook *MerkleTreeHookSession) QuoteDispatch(arg0 []byte, arg1 []byte) (*big.Int, error) {
	return _MerkleTreeHook.Contract.QuoteDispatch(&_MerkleTreeHook.CallOpts, arg0, arg1)
}

// QuoteDispatch is a free data retrieval call binding the contract method 0xaaccd230.
//
// Solidity: function quoteDispatch(bytes , bytes ) pure returns(uint256)
func (_MerkleTreeHook *MerkleTreeHookCallerSession) QuoteDispatch(arg0 []byte, arg1 []byte) (*big.Int, error) {
	return _MerkleTreeHook.Contract.QuoteDispatch(&_MerkleTreeHook.CallOpts, arg0, arg1)
}

// Root is a free data retrieval call binding the contract method 0xebf0c717.
//
// Solidity: function root() view returns(bytes32)
func (_MerkleTreeHook *MerkleTreeHookCaller) Root(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _MerkleTreeHook.contract.Call(opts, &out, "root")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// Root is a free data retrieval call binding the contract method 0xebf0c717.
//
// Solidity: function root() view returns(bytes32)
func (_MerkleTreeHook *MerkleTreeHookSession) Root() ([32]byte, error) {
	return _MerkleTreeHook.Contract.Root(&_MerkleTreeHook.CallOpts)
}

// Root is a free data retrieval call binding the contract method 0xebf0c717.
//
// Solidity: function root() view returns(bytes32)
func (_MerkleTreeHook *MerkleTreeHookCallerSession) Root() ([32]byte, error) {
	return _MerkleTreeHook.Contract.Root(&_MerkleTreeHook.CallOpts)
}

// Tree is a free data retrieval call binding the contract method 0xfd54b228.
//
// Solidity: function tree() view returns((bytes32[32],uint256))
func (_MerkleTreeHook *MerkleTreeHookCaller) Tree(opts *bind.CallOpts) (MerkleLibTree, error) {
	var out []interface{}
	err := _MerkleTreeHook.contract.Call(opts, &out, "tree")

	if err != nil {
		return *new(MerkleLibTree), err
	}

	out0 := *abi.ConvertType(out[0], new(MerkleLibTree)).(*MerkleLibTree)

	return out0, err

}

// Tree is a free data retrieval call binding the contract method 0xfd54b228.
//
// Solidity: function tree() view returns((bytes32[32],uint256))
func (_MerkleTreeHook *MerkleTreeHookSession) Tree() (MerkleLibTree, error) {
	return _MerkleTreeHook.Contract.Tree(&_MerkleTreeHook.CallOpts)
}

// Tree is a free data retrieval call binding the contract method 0xfd54b228.
//
// Solidity: function tree() view returns((bytes32[32],uint256))
func (_MerkleTreeHook *MerkleTreeHookCallerSession) Tree() (MerkleLibTree, error) {
	return _MerkleTreeHook.Contract.Tree(&_MerkleTreeHook.CallOpts)
}

// PostDispatch is a paid mutator transaction binding the contract method 0x086011b9.
//
// Solidity: function postDispatch(bytes , bytes message) payable returns()
func (_MerkleTreeHook *MerkleTreeHookTransactor) PostDispatch(opts *bind.TransactOpts, arg0 []byte, message []byte) (*types.Transaction, error) {
	return _MerkleTreeHook.contract.Transact(opts, "postDispatch", arg0, message)
}

// PostDispatch is a paid mutator transaction binding the contract method 0x086011b9.
//
// Solidity: function postDispatch(bytes , bytes message) payable returns()
func (_MerkleTreeHook *MerkleTreeHookSession) PostDispatch(arg0 []byte, message []byte) (*types.Transaction, error) {
	return _MerkleTreeHook.Contract.PostDispatch(&_MerkleTreeHook.TransactOpts, arg0, message)
}

// PostDispatch is a paid mutator transaction binding the contract method 0x086011b9.
//
// Solidity: function postDispatch(bytes , bytes message) payable returns()
func (_MerkleTreeHook *MerkleTreeHookTransactorSession) PostDispatch(arg0 []byte, message []byte) (*types.Transaction, error) {
	return _MerkleTreeHook.Contract.PostDispatch(&_MerkleTreeHook.TransactOpts, arg0, message)
}

// MerkleTreeHookInsertedIntoTreeIterator is returned from FilterInsertedIntoTree and is used to iterate over the raw logs and unpacked data for InsertedIntoTree events raised by the MerkleTreeHook contract.
type MerkleTreeHookInsertedIntoTreeIterator struct {
	Event *MerkleTreeHookInsertedIntoTree // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *MerkleTreeHookInsertedIntoTreeIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(MerkleTreeHookInsertedIntoTree)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(MerkleTreeHookInsertedIntoTree)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *MerkleTreeHookInsertedIntoTreeIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *MerkleTreeHookInsertedIntoTreeIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// MerkleTreeHookInsertedIntoTree represents a InsertedIntoTree event raised by the MerkleTreeHook contract.
type MerkleTreeHookInsertedIntoTree struct {
	MessageId [32]byte
	Index     uint32
	Raw       types.Log // Blockchain specific contextual infos
}

// FilterInsertedIntoTree is a free log retrieval operation binding the contract event 0x253a3a04cab70d47c1504809242d9350cd81627b4f1d50753e159cf8cd76ed33.
//
// Solidity: event InsertedIntoTree(bytes32 messageId, uint32 index)
func (_MerkleTreeHook *MerkleTreeHookFilterer) FilterInsertedIntoTree(opts *bind.FilterOpts) (*MerkleTreeHookInsertedIntoTreeIterator, error) {

	logs, sub, err := _MerkleTreeHook.contract.FilterLogs(opts, "InsertedIntoTree")
	if err != nil {
		return nil, err
	}
	return &MerkleTreeHookInsertedIntoTreeIterator{contract: _MerkleTreeHook.contract, event: "InsertedIntoTree", logs: logs, sub: sub}, nil
}

// WatchInsertedIntoTree is a free log subscription operation binding the contract event 0x253a3a04cab70d47c1504809242d9350cd81627b4f1d50753e159cf8cd76ed33.
//
// Solidity: event InsertedIntoTree(bytes32 messageId, uint32 index)
func (_MerkleTreeHook *MerkleTreeHookFilterer) WatchInsertedIntoTree(opts *bind.WatchOpts, sink chan<- *MerkleTreeHookInsertedIntoTree) (event.Subscription, error) {

	logs, sub, err := _MerkleTreeHook.contract.WatchLogs(opts, "InsertedIntoTree")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(MerkleTreeHookInsertedIntoTree)
				if err := _MerkleTreeHook.contract.UnpackLog(event, "InsertedIntoTree", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseInsertedIntoTree is a log parse operation binding the contract event 0x253a3a04cab70d47c1504809242d9350cd81627b4f1d50753e159cf8cd76ed33.
//
// Solidity: event InsertedIntoTree(bytes32 messageId, uint32 index)
func (_MerkleTreeHook *MerkleTreeHookFilterer) ParseInsertedIntoTree(log types.Log) (*MerkleTreeHookInsertedIntoTree, error) {
	event := new(MerkleTreeHookInsertedIntoTree)
	if err := _MerkleTreeHook.contract.UnpackLog(event, "InsertedIntoTree", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
