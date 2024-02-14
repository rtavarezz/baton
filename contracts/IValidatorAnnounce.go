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

// IValidatorAnnounceMetaData contains all meta data concerning the IValidatorAnnounce contract.
var IValidatorAnnounceMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_validator\",\"type\":\"address\"},{\"internalType\":\"string\",\"name\":\"_storageLocation\",\"type\":\"string\"},{\"internalType\":\"bytes\",\"name\":\"_signature\",\"type\":\"bytes\"}],\"name\":\"announce\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address[]\",\"name\":\"_validators\",\"type\":\"address[]\"}],\"name\":\"getAnnouncedStorageLocations\",\"outputs\":[{\"internalType\":\"string[][]\",\"name\":\"\",\"type\":\"string[][]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getAnnouncedValidators\",\"outputs\":[{\"internalType\":\"address[]\",\"name\":\"\",\"type\":\"address[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"localDomain\",\"outputs\":[{\"internalType\":\"uint32\",\"name\":\"\",\"type\":\"uint32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"mailbox\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
}

// IValidatorAnnounceABI is the input ABI used to generate the binding from.
// Deprecated: Use IValidatorAnnounceMetaData.ABI instead.
var IValidatorAnnounceABI = IValidatorAnnounceMetaData.ABI

// IValidatorAnnounce is an auto generated Go binding around an Ethereum contract.
type IValidatorAnnounce struct {
	IValidatorAnnounceCaller     // Read-only binding to the contract
	IValidatorAnnounceTransactor // Write-only binding to the contract
	IValidatorAnnounceFilterer   // Log filterer for contract events
}

// IValidatorAnnounceCaller is an auto generated read-only Go binding around an Ethereum contract.
type IValidatorAnnounceCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IValidatorAnnounceTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IValidatorAnnounceTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IValidatorAnnounceFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IValidatorAnnounceFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IValidatorAnnounceSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IValidatorAnnounceSession struct {
	Contract     *IValidatorAnnounce // Generic contract binding to set the session for
	CallOpts     bind.CallOpts       // Call options to use throughout this session
	TransactOpts bind.TransactOpts   // Transaction auth options to use throughout this session
}

// IValidatorAnnounceCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IValidatorAnnounceCallerSession struct {
	Contract *IValidatorAnnounceCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts             // Call options to use throughout this session
}

// IValidatorAnnounceTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IValidatorAnnounceTransactorSession struct {
	Contract     *IValidatorAnnounceTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts             // Transaction auth options to use throughout this session
}

// IValidatorAnnounceRaw is an auto generated low-level Go binding around an Ethereum contract.
type IValidatorAnnounceRaw struct {
	Contract *IValidatorAnnounce // Generic contract binding to access the raw methods on
}

// IValidatorAnnounceCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IValidatorAnnounceCallerRaw struct {
	Contract *IValidatorAnnounceCaller // Generic read-only contract binding to access the raw methods on
}

// IValidatorAnnounceTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IValidatorAnnounceTransactorRaw struct {
	Contract *IValidatorAnnounceTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIValidatorAnnounce creates a new instance of IValidatorAnnounce, bound to a specific deployed contract.
func NewIValidatorAnnounce(address common.Address, backend bind.ContractBackend) (*IValidatorAnnounce, error) {
	contract, err := bindIValidatorAnnounce(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IValidatorAnnounce{IValidatorAnnounceCaller: IValidatorAnnounceCaller{contract: contract}, IValidatorAnnounceTransactor: IValidatorAnnounceTransactor{contract: contract}, IValidatorAnnounceFilterer: IValidatorAnnounceFilterer{contract: contract}}, nil
}

// NewIValidatorAnnounceCaller creates a new read-only instance of IValidatorAnnounce, bound to a specific deployed contract.
func NewIValidatorAnnounceCaller(address common.Address, caller bind.ContractCaller) (*IValidatorAnnounceCaller, error) {
	contract, err := bindIValidatorAnnounce(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IValidatorAnnounceCaller{contract: contract}, nil
}

// NewIValidatorAnnounceTransactor creates a new write-only instance of IValidatorAnnounce, bound to a specific deployed contract.
func NewIValidatorAnnounceTransactor(address common.Address, transactor bind.ContractTransactor) (*IValidatorAnnounceTransactor, error) {
	contract, err := bindIValidatorAnnounce(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IValidatorAnnounceTransactor{contract: contract}, nil
}

// NewIValidatorAnnounceFilterer creates a new log filterer instance of IValidatorAnnounce, bound to a specific deployed contract.
func NewIValidatorAnnounceFilterer(address common.Address, filterer bind.ContractFilterer) (*IValidatorAnnounceFilterer, error) {
	contract, err := bindIValidatorAnnounce(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IValidatorAnnounceFilterer{contract: contract}, nil
}

// bindIValidatorAnnounce binds a generic wrapper to an already deployed contract.
func bindIValidatorAnnounce(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IValidatorAnnounceMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IValidatorAnnounce *IValidatorAnnounceRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IValidatorAnnounce.Contract.IValidatorAnnounceCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IValidatorAnnounce *IValidatorAnnounceRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IValidatorAnnounce.Contract.IValidatorAnnounceTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IValidatorAnnounce *IValidatorAnnounceRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IValidatorAnnounce.Contract.IValidatorAnnounceTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IValidatorAnnounce *IValidatorAnnounceCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IValidatorAnnounce.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IValidatorAnnounce *IValidatorAnnounceTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IValidatorAnnounce.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IValidatorAnnounce *IValidatorAnnounceTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IValidatorAnnounce.Contract.contract.Transact(opts, method, params...)
}

// GetAnnouncedStorageLocations is a free data retrieval call binding the contract method 0x51abe7cc.
//
// Solidity: function getAnnouncedStorageLocations(address[] _validators) view returns(string[][])
func (_IValidatorAnnounce *IValidatorAnnounceCaller) GetAnnouncedStorageLocations(opts *bind.CallOpts, _validators []common.Address) ([][]string, error) {
	var out []interface{}
	err := _IValidatorAnnounce.contract.Call(opts, &out, "getAnnouncedStorageLocations", _validators)

	if err != nil {
		return *new([][]string), err
	}

	out0 := *abi.ConvertType(out[0], new([][]string)).(*[][]string)

	return out0, err

}

// GetAnnouncedStorageLocations is a free data retrieval call binding the contract method 0x51abe7cc.
//
// Solidity: function getAnnouncedStorageLocations(address[] _validators) view returns(string[][])
func (_IValidatorAnnounce *IValidatorAnnounceSession) GetAnnouncedStorageLocations(_validators []common.Address) ([][]string, error) {
	return _IValidatorAnnounce.Contract.GetAnnouncedStorageLocations(&_IValidatorAnnounce.CallOpts, _validators)
}

// GetAnnouncedStorageLocations is a free data retrieval call binding the contract method 0x51abe7cc.
//
// Solidity: function getAnnouncedStorageLocations(address[] _validators) view returns(string[][])
func (_IValidatorAnnounce *IValidatorAnnounceCallerSession) GetAnnouncedStorageLocations(_validators []common.Address) ([][]string, error) {
	return _IValidatorAnnounce.Contract.GetAnnouncedStorageLocations(&_IValidatorAnnounce.CallOpts, _validators)
}

// GetAnnouncedValidators is a free data retrieval call binding the contract method 0x690cb786.
//
// Solidity: function getAnnouncedValidators() view returns(address[])
func (_IValidatorAnnounce *IValidatorAnnounceCaller) GetAnnouncedValidators(opts *bind.CallOpts) ([]common.Address, error) {
	var out []interface{}
	err := _IValidatorAnnounce.contract.Call(opts, &out, "getAnnouncedValidators")

	if err != nil {
		return *new([]common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new([]common.Address)).(*[]common.Address)

	return out0, err

}

// GetAnnouncedValidators is a free data retrieval call binding the contract method 0x690cb786.
//
// Solidity: function getAnnouncedValidators() view returns(address[])
func (_IValidatorAnnounce *IValidatorAnnounceSession) GetAnnouncedValidators() ([]common.Address, error) {
	return _IValidatorAnnounce.Contract.GetAnnouncedValidators(&_IValidatorAnnounce.CallOpts)
}

// GetAnnouncedValidators is a free data retrieval call binding the contract method 0x690cb786.
//
// Solidity: function getAnnouncedValidators() view returns(address[])
func (_IValidatorAnnounce *IValidatorAnnounceCallerSession) GetAnnouncedValidators() ([]common.Address, error) {
	return _IValidatorAnnounce.Contract.GetAnnouncedValidators(&_IValidatorAnnounce.CallOpts)
}

// LocalDomain is a free data retrieval call binding the contract method 0x8d3638f4.
//
// Solidity: function localDomain() view returns(uint32)
func (_IValidatorAnnounce *IValidatorAnnounceCaller) LocalDomain(opts *bind.CallOpts) (uint32, error) {
	var out []interface{}
	err := _IValidatorAnnounce.contract.Call(opts, &out, "localDomain")

	if err != nil {
		return *new(uint32), err
	}

	out0 := *abi.ConvertType(out[0], new(uint32)).(*uint32)

	return out0, err

}

// LocalDomain is a free data retrieval call binding the contract method 0x8d3638f4.
//
// Solidity: function localDomain() view returns(uint32)
func (_IValidatorAnnounce *IValidatorAnnounceSession) LocalDomain() (uint32, error) {
	return _IValidatorAnnounce.Contract.LocalDomain(&_IValidatorAnnounce.CallOpts)
}

// LocalDomain is a free data retrieval call binding the contract method 0x8d3638f4.
//
// Solidity: function localDomain() view returns(uint32)
func (_IValidatorAnnounce *IValidatorAnnounceCallerSession) LocalDomain() (uint32, error) {
	return _IValidatorAnnounce.Contract.LocalDomain(&_IValidatorAnnounce.CallOpts)
}

// Mailbox is a free data retrieval call binding the contract method 0xd5438eae.
//
// Solidity: function mailbox() view returns(address)
func (_IValidatorAnnounce *IValidatorAnnounceCaller) Mailbox(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IValidatorAnnounce.contract.Call(opts, &out, "mailbox")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Mailbox is a free data retrieval call binding the contract method 0xd5438eae.
//
// Solidity: function mailbox() view returns(address)
func (_IValidatorAnnounce *IValidatorAnnounceSession) Mailbox() (common.Address, error) {
	return _IValidatorAnnounce.Contract.Mailbox(&_IValidatorAnnounce.CallOpts)
}

// Mailbox is a free data retrieval call binding the contract method 0xd5438eae.
//
// Solidity: function mailbox() view returns(address)
func (_IValidatorAnnounce *IValidatorAnnounceCallerSession) Mailbox() (common.Address, error) {
	return _IValidatorAnnounce.Contract.Mailbox(&_IValidatorAnnounce.CallOpts)
}

// Announce is a paid mutator transaction binding the contract method 0x21f71781.
//
// Solidity: function announce(address _validator, string _storageLocation, bytes _signature) returns(bool)
func (_IValidatorAnnounce *IValidatorAnnounceTransactor) Announce(opts *bind.TransactOpts, _validator common.Address, _storageLocation string, _signature []byte) (*types.Transaction, error) {
	return _IValidatorAnnounce.contract.Transact(opts, "announce", _validator, _storageLocation, _signature)
}

// Announce is a paid mutator transaction binding the contract method 0x21f71781.
//
// Solidity: function announce(address _validator, string _storageLocation, bytes _signature) returns(bool)
func (_IValidatorAnnounce *IValidatorAnnounceSession) Announce(_validator common.Address, _storageLocation string, _signature []byte) (*types.Transaction, error) {
	return _IValidatorAnnounce.Contract.Announce(&_IValidatorAnnounce.TransactOpts, _validator, _storageLocation, _signature)
}

// Announce is a paid mutator transaction binding the contract method 0x21f71781.
//
// Solidity: function announce(address _validator, string _storageLocation, bytes _signature) returns(bool)
func (_IValidatorAnnounce *IValidatorAnnounceTransactorSession) Announce(_validator common.Address, _storageLocation string, _signature []byte) (*types.Transaction, error) {
	return _IValidatorAnnounce.Contract.Announce(&_IValidatorAnnounce.TransactOpts, _validator, _storageLocation, _signature)
}
