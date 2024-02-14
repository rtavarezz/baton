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

// IInterchainGasPaymasterMetaData contains all meta data concerning the IInterchainGasPaymaster contract.
var IInterchainGasPaymasterMetaData = &bind.MetaData{
	ABI: "[{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"messageId\",\"type\":\"bytes32\"},{\"indexed\":true,\"internalType\":\"uint32\",\"name\":\"destinationDomain\",\"type\":\"uint32\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"gasAmount\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"payment\",\"type\":\"uint256\"}],\"name\":\"GasPayment\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"_messageId\",\"type\":\"bytes32\"},{\"internalType\":\"uint32\",\"name\":\"_destinationDomain\",\"type\":\"uint32\"},{\"internalType\":\"uint256\",\"name\":\"_gasAmount\",\"type\":\"uint256\"},{\"internalType\":\"address\",\"name\":\"_refundAddress\",\"type\":\"address\"}],\"name\":\"payForGas\",\"outputs\":[],\"stateMutability\":\"payable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint32\",\"name\":\"_destinationDomain\",\"type\":\"uint32\"},{\"internalType\":\"uint256\",\"name\":\"_gasAmount\",\"type\":\"uint256\"}],\"name\":\"quoteGasPayment\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
}

// IInterchainGasPaymasterABI is the input ABI used to generate the binding from.
// Deprecated: Use IInterchainGasPaymasterMetaData.ABI instead.
var IInterchainGasPaymasterABI = IInterchainGasPaymasterMetaData.ABI

// IInterchainGasPaymaster is an auto generated Go binding around an Ethereum contract.
type IInterchainGasPaymaster struct {
	IInterchainGasPaymasterCaller     // Read-only binding to the contract
	IInterchainGasPaymasterTransactor // Write-only binding to the contract
	IInterchainGasPaymasterFilterer   // Log filterer for contract events
}

// IInterchainGasPaymasterCaller is an auto generated read-only Go binding around an Ethereum contract.
type IInterchainGasPaymasterCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IInterchainGasPaymasterTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IInterchainGasPaymasterTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IInterchainGasPaymasterFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IInterchainGasPaymasterFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IInterchainGasPaymasterSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IInterchainGasPaymasterSession struct {
	Contract     *IInterchainGasPaymaster // Generic contract binding to set the session for
	CallOpts     bind.CallOpts            // Call options to use throughout this session
	TransactOpts bind.TransactOpts        // Transaction auth options to use throughout this session
}

// IInterchainGasPaymasterCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IInterchainGasPaymasterCallerSession struct {
	Contract *IInterchainGasPaymasterCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts                  // Call options to use throughout this session
}

// IInterchainGasPaymasterTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IInterchainGasPaymasterTransactorSession struct {
	Contract     *IInterchainGasPaymasterTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts                  // Transaction auth options to use throughout this session
}

// IInterchainGasPaymasterRaw is an auto generated low-level Go binding around an Ethereum contract.
type IInterchainGasPaymasterRaw struct {
	Contract *IInterchainGasPaymaster // Generic contract binding to access the raw methods on
}

// IInterchainGasPaymasterCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IInterchainGasPaymasterCallerRaw struct {
	Contract *IInterchainGasPaymasterCaller // Generic read-only contract binding to access the raw methods on
}

// IInterchainGasPaymasterTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IInterchainGasPaymasterTransactorRaw struct {
	Contract *IInterchainGasPaymasterTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIInterchainGasPaymaster creates a new instance of IInterchainGasPaymaster, bound to a specific deployed contract.
func NewIInterchainGasPaymaster(address common.Address, backend bind.ContractBackend) (*IInterchainGasPaymaster, error) {
	contract, err := bindIInterchainGasPaymaster(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IInterchainGasPaymaster{IInterchainGasPaymasterCaller: IInterchainGasPaymasterCaller{contract: contract}, IInterchainGasPaymasterTransactor: IInterchainGasPaymasterTransactor{contract: contract}, IInterchainGasPaymasterFilterer: IInterchainGasPaymasterFilterer{contract: contract}}, nil
}

// NewIInterchainGasPaymasterCaller creates a new read-only instance of IInterchainGasPaymaster, bound to a specific deployed contract.
func NewIInterchainGasPaymasterCaller(address common.Address, caller bind.ContractCaller) (*IInterchainGasPaymasterCaller, error) {
	contract, err := bindIInterchainGasPaymaster(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IInterchainGasPaymasterCaller{contract: contract}, nil
}

// NewIInterchainGasPaymasterTransactor creates a new write-only instance of IInterchainGasPaymaster, bound to a specific deployed contract.
func NewIInterchainGasPaymasterTransactor(address common.Address, transactor bind.ContractTransactor) (*IInterchainGasPaymasterTransactor, error) {
	contract, err := bindIInterchainGasPaymaster(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IInterchainGasPaymasterTransactor{contract: contract}, nil
}

// NewIInterchainGasPaymasterFilterer creates a new log filterer instance of IInterchainGasPaymaster, bound to a specific deployed contract.
func NewIInterchainGasPaymasterFilterer(address common.Address, filterer bind.ContractFilterer) (*IInterchainGasPaymasterFilterer, error) {
	contract, err := bindIInterchainGasPaymaster(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IInterchainGasPaymasterFilterer{contract: contract}, nil
}

// bindIInterchainGasPaymaster binds a generic wrapper to an already deployed contract.
func bindIInterchainGasPaymaster(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IInterchainGasPaymasterMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IInterchainGasPaymaster *IInterchainGasPaymasterRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IInterchainGasPaymaster.Contract.IInterchainGasPaymasterCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IInterchainGasPaymaster *IInterchainGasPaymasterRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IInterchainGasPaymaster.Contract.IInterchainGasPaymasterTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IInterchainGasPaymaster *IInterchainGasPaymasterRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IInterchainGasPaymaster.Contract.IInterchainGasPaymasterTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IInterchainGasPaymaster *IInterchainGasPaymasterCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IInterchainGasPaymaster.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IInterchainGasPaymaster *IInterchainGasPaymasterTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IInterchainGasPaymaster.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IInterchainGasPaymaster *IInterchainGasPaymasterTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IInterchainGasPaymaster.Contract.contract.Transact(opts, method, params...)
}

// QuoteGasPayment is a free data retrieval call binding the contract method 0xa6929793.
//
// Solidity: function quoteGasPayment(uint32 _destinationDomain, uint256 _gasAmount) view returns(uint256)
func (_IInterchainGasPaymaster *IInterchainGasPaymasterCaller) QuoteGasPayment(opts *bind.CallOpts, _destinationDomain uint32, _gasAmount *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _IInterchainGasPaymaster.contract.Call(opts, &out, "quoteGasPayment", _destinationDomain, _gasAmount)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// QuoteGasPayment is a free data retrieval call binding the contract method 0xa6929793.
//
// Solidity: function quoteGasPayment(uint32 _destinationDomain, uint256 _gasAmount) view returns(uint256)
func (_IInterchainGasPaymaster *IInterchainGasPaymasterSession) QuoteGasPayment(_destinationDomain uint32, _gasAmount *big.Int) (*big.Int, error) {
	return _IInterchainGasPaymaster.Contract.QuoteGasPayment(&_IInterchainGasPaymaster.CallOpts, _destinationDomain, _gasAmount)
}

// QuoteGasPayment is a free data retrieval call binding the contract method 0xa6929793.
//
// Solidity: function quoteGasPayment(uint32 _destinationDomain, uint256 _gasAmount) view returns(uint256)
func (_IInterchainGasPaymaster *IInterchainGasPaymasterCallerSession) QuoteGasPayment(_destinationDomain uint32, _gasAmount *big.Int) (*big.Int, error) {
	return _IInterchainGasPaymaster.Contract.QuoteGasPayment(&_IInterchainGasPaymaster.CallOpts, _destinationDomain, _gasAmount)
}

// PayForGas is a paid mutator transaction binding the contract method 0x11bf2c18.
//
// Solidity: function payForGas(bytes32 _messageId, uint32 _destinationDomain, uint256 _gasAmount, address _refundAddress) payable returns()
func (_IInterchainGasPaymaster *IInterchainGasPaymasterTransactor) PayForGas(opts *bind.TransactOpts, _messageId [32]byte, _destinationDomain uint32, _gasAmount *big.Int, _refundAddress common.Address) (*types.Transaction, error) {
	return _IInterchainGasPaymaster.contract.Transact(opts, "payForGas", _messageId, _destinationDomain, _gasAmount, _refundAddress)
}

// PayForGas is a paid mutator transaction binding the contract method 0x11bf2c18.
//
// Solidity: function payForGas(bytes32 _messageId, uint32 _destinationDomain, uint256 _gasAmount, address _refundAddress) payable returns()
func (_IInterchainGasPaymaster *IInterchainGasPaymasterSession) PayForGas(_messageId [32]byte, _destinationDomain uint32, _gasAmount *big.Int, _refundAddress common.Address) (*types.Transaction, error) {
	return _IInterchainGasPaymaster.Contract.PayForGas(&_IInterchainGasPaymaster.TransactOpts, _messageId, _destinationDomain, _gasAmount, _refundAddress)
}

// PayForGas is a paid mutator transaction binding the contract method 0x11bf2c18.
//
// Solidity: function payForGas(bytes32 _messageId, uint32 _destinationDomain, uint256 _gasAmount, address _refundAddress) payable returns()
func (_IInterchainGasPaymaster *IInterchainGasPaymasterTransactorSession) PayForGas(_messageId [32]byte, _destinationDomain uint32, _gasAmount *big.Int, _refundAddress common.Address) (*types.Transaction, error) {
	return _IInterchainGasPaymaster.Contract.PayForGas(&_IInterchainGasPaymaster.TransactOpts, _messageId, _destinationDomain, _gasAmount, _refundAddress)
}

// IInterchainGasPaymasterGasPaymentIterator is returned from FilterGasPayment and is used to iterate over the raw logs and unpacked data for GasPayment events raised by the IInterchainGasPaymaster contract.
type IInterchainGasPaymasterGasPaymentIterator struct {
	Event *IInterchainGasPaymasterGasPayment // Event containing the contract specifics and raw log

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
func (it *IInterchainGasPaymasterGasPaymentIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IInterchainGasPaymasterGasPayment)
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
		it.Event = new(IInterchainGasPaymasterGasPayment)
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
func (it *IInterchainGasPaymasterGasPaymentIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IInterchainGasPaymasterGasPaymentIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IInterchainGasPaymasterGasPayment represents a GasPayment event raised by the IInterchainGasPaymaster contract.
type IInterchainGasPaymasterGasPayment struct {
	MessageId         [32]byte
	DestinationDomain uint32
	GasAmount         *big.Int
	Payment           *big.Int
	Raw               types.Log // Blockchain specific contextual infos
}

// FilterGasPayment is a free log retrieval operation binding the contract event 0x65695c3748edae85a24cc2c60b299b31f463050bc259150d2e5802ec8d11720a.
//
// Solidity: event GasPayment(bytes32 indexed messageId, uint32 indexed destinationDomain, uint256 gasAmount, uint256 payment)
func (_IInterchainGasPaymaster *IInterchainGasPaymasterFilterer) FilterGasPayment(opts *bind.FilterOpts, messageId [][32]byte, destinationDomain []uint32) (*IInterchainGasPaymasterGasPaymentIterator, error) {

	var messageIdRule []interface{}
	for _, messageIdItem := range messageId {
		messageIdRule = append(messageIdRule, messageIdItem)
	}
	var destinationDomainRule []interface{}
	for _, destinationDomainItem := range destinationDomain {
		destinationDomainRule = append(destinationDomainRule, destinationDomainItem)
	}

	logs, sub, err := _IInterchainGasPaymaster.contract.FilterLogs(opts, "GasPayment", messageIdRule, destinationDomainRule)
	if err != nil {
		return nil, err
	}
	return &IInterchainGasPaymasterGasPaymentIterator{contract: _IInterchainGasPaymaster.contract, event: "GasPayment", logs: logs, sub: sub}, nil
}

// WatchGasPayment is a free log subscription operation binding the contract event 0x65695c3748edae85a24cc2c60b299b31f463050bc259150d2e5802ec8d11720a.
//
// Solidity: event GasPayment(bytes32 indexed messageId, uint32 indexed destinationDomain, uint256 gasAmount, uint256 payment)
func (_IInterchainGasPaymaster *IInterchainGasPaymasterFilterer) WatchGasPayment(opts *bind.WatchOpts, sink chan<- *IInterchainGasPaymasterGasPayment, messageId [][32]byte, destinationDomain []uint32) (event.Subscription, error) {

	var messageIdRule []interface{}
	for _, messageIdItem := range messageId {
		messageIdRule = append(messageIdRule, messageIdItem)
	}
	var destinationDomainRule []interface{}
	for _, destinationDomainItem := range destinationDomain {
		destinationDomainRule = append(destinationDomainRule, destinationDomainItem)
	}

	logs, sub, err := _IInterchainGasPaymaster.contract.WatchLogs(opts, "GasPayment", messageIdRule, destinationDomainRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IInterchainGasPaymasterGasPayment)
				if err := _IInterchainGasPaymaster.contract.UnpackLog(event, "GasPayment", log); err != nil {
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

// ParseGasPayment is a log parse operation binding the contract event 0x65695c3748edae85a24cc2c60b299b31f463050bc259150d2e5802ec8d11720a.
//
// Solidity: event GasPayment(bytes32 indexed messageId, uint32 indexed destinationDomain, uint256 gasAmount, uint256 payment)
func (_IInterchainGasPaymaster *IInterchainGasPaymasterFilterer) ParseGasPayment(log types.Log) (*IInterchainGasPaymasterGasPayment, error) {
	event := new(IInterchainGasPaymasterGasPayment)
	if err := _IInterchainGasPaymaster.contract.UnpackLog(event, "GasPayment", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
