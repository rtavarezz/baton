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

// IMultisigIsmMetaData contains all meta data concerning the IMultisigIsm contract.
var IMultisigIsmMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[],\"name\":\"moduleType\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes\",\"name\":\"_message\",\"type\":\"bytes\"}],\"name\":\"validatorsAndThreshold\",\"outputs\":[{\"internalType\":\"address[]\",\"name\":\"validators\",\"type\":\"address[]\"},{\"internalType\":\"uint8\",\"name\":\"threshold\",\"type\":\"uint8\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes\",\"name\":\"_metadata\",\"type\":\"bytes\"},{\"internalType\":\"bytes\",\"name\":\"_message\",\"type\":\"bytes\"}],\"name\":\"verify\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
}

// IMultisigIsmABI is the input ABI used to generate the binding from.
// Deprecated: Use IMultisigIsmMetaData.ABI instead.
var IMultisigIsmABI = IMultisigIsmMetaData.ABI

// IMultisigIsm is an auto generated Go binding around an Ethereum contract.
type IMultisigIsm struct {
	IMultisigIsmCaller     // Read-only binding to the contract
	IMultisigIsmTransactor // Write-only binding to the contract
	IMultisigIsmFilterer   // Log filterer for contract events
}

// IMultisigIsmCaller is an auto generated read-only Go binding around an Ethereum contract.
type IMultisigIsmCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IMultisigIsmTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IMultisigIsmTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IMultisigIsmFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IMultisigIsmFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IMultisigIsmSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IMultisigIsmSession struct {
	Contract     *IMultisigIsm     // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// IMultisigIsmCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IMultisigIsmCallerSession struct {
	Contract *IMultisigIsmCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts       // Call options to use throughout this session
}

// IMultisigIsmTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IMultisigIsmTransactorSession struct {
	Contract     *IMultisigIsmTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts       // Transaction auth options to use throughout this session
}

// IMultisigIsmRaw is an auto generated low-level Go binding around an Ethereum contract.
type IMultisigIsmRaw struct {
	Contract *IMultisigIsm // Generic contract binding to access the raw methods on
}

// IMultisigIsmCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IMultisigIsmCallerRaw struct {
	Contract *IMultisigIsmCaller // Generic read-only contract binding to access the raw methods on
}

// IMultisigIsmTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IMultisigIsmTransactorRaw struct {
	Contract *IMultisigIsmTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIMultisigIsm creates a new instance of IMultisigIsm, bound to a specific deployed contract.
func NewIMultisigIsm(address common.Address, backend bind.ContractBackend) (*IMultisigIsm, error) {
	contract, err := bindIMultisigIsm(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IMultisigIsm{IMultisigIsmCaller: IMultisigIsmCaller{contract: contract}, IMultisigIsmTransactor: IMultisigIsmTransactor{contract: contract}, IMultisigIsmFilterer: IMultisigIsmFilterer{contract: contract}}, nil
}

// NewIMultisigIsmCaller creates a new read-only instance of IMultisigIsm, bound to a specific deployed contract.
func NewIMultisigIsmCaller(address common.Address, caller bind.ContractCaller) (*IMultisigIsmCaller, error) {
	contract, err := bindIMultisigIsm(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IMultisigIsmCaller{contract: contract}, nil
}

// NewIMultisigIsmTransactor creates a new write-only instance of IMultisigIsm, bound to a specific deployed contract.
func NewIMultisigIsmTransactor(address common.Address, transactor bind.ContractTransactor) (*IMultisigIsmTransactor, error) {
	contract, err := bindIMultisigIsm(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IMultisigIsmTransactor{contract: contract}, nil
}

// NewIMultisigIsmFilterer creates a new log filterer instance of IMultisigIsm, bound to a specific deployed contract.
func NewIMultisigIsmFilterer(address common.Address, filterer bind.ContractFilterer) (*IMultisigIsmFilterer, error) {
	contract, err := bindIMultisigIsm(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IMultisigIsmFilterer{contract: contract}, nil
}

// bindIMultisigIsm binds a generic wrapper to an already deployed contract.
func bindIMultisigIsm(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IMultisigIsmMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IMultisigIsm *IMultisigIsmRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IMultisigIsm.Contract.IMultisigIsmCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IMultisigIsm *IMultisigIsmRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IMultisigIsm.Contract.IMultisigIsmTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IMultisigIsm *IMultisigIsmRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IMultisigIsm.Contract.IMultisigIsmTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IMultisigIsm *IMultisigIsmCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IMultisigIsm.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IMultisigIsm *IMultisigIsmTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IMultisigIsm.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IMultisigIsm *IMultisigIsmTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IMultisigIsm.Contract.contract.Transact(opts, method, params...)
}

// ModuleType is a free data retrieval call binding the contract method 0x6465e69f.
//
// Solidity: function moduleType() view returns(uint8)
func (_IMultisigIsm *IMultisigIsmCaller) ModuleType(opts *bind.CallOpts) (uint8, error) {
	var out []interface{}
	err := _IMultisigIsm.contract.Call(opts, &out, "moduleType")

	if err != nil {
		return *new(uint8), err
	}

	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)

	return out0, err

}

// ModuleType is a free data retrieval call binding the contract method 0x6465e69f.
//
// Solidity: function moduleType() view returns(uint8)
func (_IMultisigIsm *IMultisigIsmSession) ModuleType() (uint8, error) {
	return _IMultisigIsm.Contract.ModuleType(&_IMultisigIsm.CallOpts)
}

// ModuleType is a free data retrieval call binding the contract method 0x6465e69f.
//
// Solidity: function moduleType() view returns(uint8)
func (_IMultisigIsm *IMultisigIsmCallerSession) ModuleType() (uint8, error) {
	return _IMultisigIsm.Contract.ModuleType(&_IMultisigIsm.CallOpts)
}

// ValidatorsAndThreshold is a free data retrieval call binding the contract method 0x2e0ed234.
//
// Solidity: function validatorsAndThreshold(bytes _message) view returns(address[] validators, uint8 threshold)
func (_IMultisigIsm *IMultisigIsmCaller) ValidatorsAndThreshold(opts *bind.CallOpts, _message []byte) (struct {
	Validators []common.Address
	Threshold  uint8
}, error) {
	var out []interface{}
	err := _IMultisigIsm.contract.Call(opts, &out, "validatorsAndThreshold", _message)

	outstruct := new(struct {
		Validators []common.Address
		Threshold  uint8
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Validators = *abi.ConvertType(out[0], new([]common.Address)).(*[]common.Address)
	outstruct.Threshold = *abi.ConvertType(out[1], new(uint8)).(*uint8)

	return *outstruct, err

}

// ValidatorsAndThreshold is a free data retrieval call binding the contract method 0x2e0ed234.
//
// Solidity: function validatorsAndThreshold(bytes _message) view returns(address[] validators, uint8 threshold)
func (_IMultisigIsm *IMultisigIsmSession) ValidatorsAndThreshold(_message []byte) (struct {
	Validators []common.Address
	Threshold  uint8
}, error) {
	return _IMultisigIsm.Contract.ValidatorsAndThreshold(&_IMultisigIsm.CallOpts, _message)
}

// ValidatorsAndThreshold is a free data retrieval call binding the contract method 0x2e0ed234.
//
// Solidity: function validatorsAndThreshold(bytes _message) view returns(address[] validators, uint8 threshold)
func (_IMultisigIsm *IMultisigIsmCallerSession) ValidatorsAndThreshold(_message []byte) (struct {
	Validators []common.Address
	Threshold  uint8
}, error) {
	return _IMultisigIsm.Contract.ValidatorsAndThreshold(&_IMultisigIsm.CallOpts, _message)
}

// Verify is a paid mutator transaction binding the contract method 0xf7e83aee.
//
// Solidity: function verify(bytes _metadata, bytes _message) returns(bool)
func (_IMultisigIsm *IMultisigIsmTransactor) Verify(opts *bind.TransactOpts, _metadata []byte, _message []byte) (*types.Transaction, error) {
	return _IMultisigIsm.contract.Transact(opts, "verify", _metadata, _message)
}

// Verify is a paid mutator transaction binding the contract method 0xf7e83aee.
//
// Solidity: function verify(bytes _metadata, bytes _message) returns(bool)
func (_IMultisigIsm *IMultisigIsmSession) Verify(_metadata []byte, _message []byte) (*types.Transaction, error) {
	return _IMultisigIsm.Contract.Verify(&_IMultisigIsm.TransactOpts, _metadata, _message)
}

// Verify is a paid mutator transaction binding the contract method 0xf7e83aee.
//
// Solidity: function verify(bytes _metadata, bytes _message) returns(bool)
func (_IMultisigIsm *IMultisigIsmTransactorSession) Verify(_metadata []byte, _message []byte) (*types.Transaction, error) {
	return _IMultisigIsm.Contract.Verify(&_IMultisigIsm.TransactOpts, _metadata, _message)
}
