package database

import (
	"encoding/json"
	"errors"
	"github.com/flashbots/mev-boost-relay/common"
)

var ErrUnsupportedExecutionPayload = errors.New("unsupported execution payload version")

func AnchorPayloadToExecPayloadEntry(
	payload *common.AnchorPayload,
	blockReq *common.SubmitNewBlockRequest,
) (*ExecutionPayloadEntry, error) {
	var _payload []byte
	var version string
	var err error

	if payload != nil {
		_payload, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}

	return &ExecutionPayloadEntry{
		Slot:           payload.Slot,
		ProposerPubkey: blockReq.ProposerPubKey().String(),
		BlockHash:      blockReq.BlockHash().String(),
		Version:        version,
		Payload:        string(_payload),
	}, nil
}

func DeliveredPayloadEntryToBidTraceV2JSON(payload *DeliveredPayloadEntry2) common.BidTraceV2JSON {
	return common.BidTraceV2JSON{
		Slot:                 payload.Slot,
		ParentHash:           payload.ParentHash,
		BlockHash:            payload.BlockHash,
		BuilderPubkey:        payload.BuilderPubkey,
		ProposerPubkey:       payload.ProposerPubkey,
		ProposerFeeRecipient: payload.ProposerFeeRecipient,
		GasLimit:             payload.GasLimit,
		GasUsed:              payload.GasUsed,
		Value:                payload.Value,
		NumTx:                payload.NumTx,
		BlockNumber:          payload.BlockNumber,
	}
}

func BuilderSubmissionEntryToBidTraceV2WithTimestampJSON(payload *BuilderBlockSubmissionEntry) common.BidTraceV2WithTimestampJSON {
	timestamp := payload.InsertedAt
	if payload.ReceivedAt.Valid {
		timestamp = payload.ReceivedAt.Time
	}

	// TODO: Fix the below later
	/*
	   blockNumberStr, err := json.Marshal(payload.BlockNumber)
	   if err != nil {
	     log.Error("BuilderSubmissionEntryToBidTraceV2WithTimestampJSON could not marshal block number. Using default block num.")
	     blockNumberStr = []byte("")
	   }
	*/
	blockNumberStr := 100

	return common.BidTraceV2WithTimestampJSON{
		Timestamp:            timestamp.Unix(),
		TimestampMs:          timestamp.UnixMilli(),
		OptimisticSubmission: payload.OptimisticSubmission,
		BidTraceV2JSON: common.BidTraceV2JSON{
			Slot:                 payload.Slot,
			ParentHash:           payload.ParentHash,
			BlockHash:            payload.BlockHash,
			BuilderPubkey:        payload.BuilderPubkey,
			ProposerPubkey:       payload.ProposerPubkey,
			ProposerFeeRecipient: payload.ProposerFeeRecipient,
			GasLimit:             payload.GasLimit,
			GasUsed:              payload.GasUsed,
			Value:                payload.Value,
			NumTx:                payload.NumTx,
			BlockNumber:          uint64(blockNumberStr),
		},
	}
}

/*
func ExecutionPayloadEntryToExecutionPayload(executionPayloadEntry *ExecutionPayloadEntry) (payload *common.VersionedExecutionPayload, err error) {
	payloadVersion := executionPayloadEntry.Version
	if payloadVersion == common.ForkVersionStringDeneb {
		return nil, ErrUnsupportedExecutionPayload
	} else if payloadVersion == common.ForkVersionStringCapella {
		executionPayload := new(capella.ExecutionPayload)
		err = json.Unmarshal([]byte(executionPayloadEntry.Payload), executionPayload)
		if err != nil {
			return nil, err
		}
		capella := api.VersionedExecutionPayload{ //nolint:exhaustruct
			Version: consensusspec.DataVersionCapella,
			Capella: executionPayload,
		}
		return &common.VersionedExecutionPayload{ //nolint:exhaustruct
			Capella: &capella,
		}, nil
	} else if payloadVersion == common.ForkVersionStringBellatrix {
		executionPayload := new(types.ExecutionPayload)
		err = json.Unmarshal([]byte(executionPayloadEntry.Payload), executionPayload)
		if err != nil {
			return nil, err
		}
		bellatrix := types.GetPayloadResponse{
			Version: types.VersionString(common.ForkVersionStringBellatrix),
			Data:    executionPayload,
		}
		return &common.VersionedExecutionPayload{
			Bellatrix: &bellatrix,
			Capella:   nil,
		}, nil
	} else {
		return nil, ErrUnsupportedExecutionPayload
	}
}
*/

func ExecutionPayloadEntryToAnchorPayload(
	executionPayloadEntry *ExecutionPayloadEntry,
) (ret *common.AnchorPayload, err error) {
	var payload common.AnchorPayload
	if err = json.Unmarshal([]byte(executionPayloadEntry.Payload), &payload); err != nil {
		return nil, errors.New("could not unmarshal execution payload to anchor payload")
	}
	return &payload, nil
}
