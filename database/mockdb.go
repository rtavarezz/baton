package database

import (
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/AnomalyFi/baton/common"
)

type MockDB struct {
	ExecPayloads     map[string]*ExecutionPayloadEntry
	BlockSubmissions map[string]*BuilderBlockSubmissionEntry
	Builders         map[string]*BlockBuilderEntry
	Demotions        map[string]bool
	Refunds          map[string]bool
	IncludedTobTxs   map[string][]*IncludedTobTxEntry
	TobSubmitProfile map[string]*ToBSubmitProfileEntry
	RobSubmitProfile map[string]*RoBSubmitProfileEntry
}

func (db MockDB) NumRegisteredValidators() (count uint64, err error) {
	return 0, nil
}

func (db MockDB) SaveValidatorRegistration(entry ValidatorRegistrationEntry) error {
	return nil
}

func (db MockDB) GetValidatorRegistration(pubkey string) (*ValidatorRegistrationEntry, error) {
	return nil, nil
}

func (db MockDB) GetValidatorRegistrationsForPubkeys(pubkeys []string) (entries []*ValidatorRegistrationEntry, err error) {
	return nil, nil
}

func (db MockDB) GetLatestValidatorRegistrations(timestampOnly bool) ([]*ValidatorRegistrationEntry, error) {
	return nil, nil
}

/*
func (db MockDB) SaveBuilderBlockSubmission(
	payload *common.BuilderSubmitBlockRequest,
	requestError,
	validationError error,
	receivedAt,
	eligibleAt time.Time,
	wasSimulated,
	saveExecPayload bool,
	profile common.Profile,
	optimisticSubmission bool) (entry *BuilderBlockSubmissionEntry, err error) {
*/

func (db MockDB) SaveBuilderBlockSubmission(
	blockReq *common.SubmitNewBlockRequest,
	payload *common.AnchorPayload,
	gasUsed uint64,
	gasLimit uint64,
	isToB bool,
	value *big.Int,
	robChainID string,
	requestError,
	validationError error,
	receivedAt,
	eligibleAt time.Time,
	wasSimulated, saveExecPayload bool,
	profile common.Profile,
	optimisticSubmission bool,
) (entry *BuilderBlockSubmissionEntry, err error) {
	key := fmt.Sprintf("%d-%s-%s", payload.Slot, blockReq.ProposerPubKeyAsStr(), blockReq.BlockHash().String())

	execPayloadEntry, err := AnchorPayloadToExecPayloadEntry(payload, blockReq)
	if err != nil {
		return nil, err
	}

	if saveExecPayload {
		db.ExecPayloads[key] = execPayloadEntry
	}

	// Save block_submission
	simErrStr := ""
	if validationError != nil {
		simErrStr = validationError.Error()
	}

	requestErrStr := ""
	if requestError != nil {
		requestErrStr = requestError.Error()
	}

	blockNumberStr, err := blockReq.BlockNumberAsStr()
	if err != nil {
		return nil, errors.New("")
	}

	blockSubmissionEntry := &BuilderBlockSubmissionEntry{
		ReceivedAt:         NewNullTime(receivedAt),
		EligibleAt:         NewNullTime(eligibleAt),
		ExecutionPayloadID: NewNullInt64(execPayloadEntry.ID),

		WasSimulated: wasSimulated,
		SimSuccess:   wasSimulated && validationError == nil,
		SimError:     simErrStr,
		SimReqError:  requestErrStr,

		Signature: blockReq.Sig().String(),

		Slot:       payload.Slot,
		BlockHash:  blockReq.BlockHash().String(),
		ParentHash: blockReq.ParentHash().String(),

		BuilderPubkey:        blockReq.BuilderPubkey().String(),
		ProposerPubkey:       blockReq.ProposerPubKeyAsStr(),
		ProposerFeeRecipient: blockReq.ProposerPaymentAsStr(),

		GasUsed:  gasUsed,
		GasLimit: gasLimit,

		NumTx: uint64(len(payload.Transactions)),
		Value: value.String(),

		Epoch:       blockReq.Slot() / common.SlotsPerEpoch,
		BlockNumber: blockNumberStr,

		DecodeDuration:       profile.Decode,
		PrechecksDuration:    profile.Prechecks,
		SimulationDuration:   profile.Simulation,
		RedisUpdateDuration:  profile.RedisUpdate,
		TotalDuration:        profile.Total,
		OptimisticSubmission: optimisticSubmission,
	}
	db.BlockSubmissions[key] = blockSubmissionEntry
	return blockSubmissionEntry, err
}

func (db MockDB) SaveDeliveredAnchorPayload(
	bidTrace *common.BidTraceV3,
	payloadResp *common.AnchorGetPayloadResponse,
	signedAt time.Time,
	publishMs uint64,
) error {
	return nil
}

func (db MockDB) GetExecutionPayloadEntryByID(executionPayloadID int64) (entry *ExecutionPayloadEntry, err error) {
	return nil, nil
}

func (db MockDB) GetExecutionPayloadEntryBySlotPkHash(slot uint64, proposerPubkey, blockHash string) (entry *ExecutionPayloadEntry, err error) {
	key := fmt.Sprintf("%d-%s-%s", slot, proposerPubkey, blockHash)
	entry, ok := db.ExecPayloads[key]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return entry, nil
}

func (db MockDB) GetToBAnchorPayloadEntryBySlotPkHash(slot uint64, proposerPubkey, blockHash string) (entry *ExecutionPayloadEntry, err error) {
	key := fmt.Sprintf("tob,%d-%s-%s", slot, proposerPubkey, blockHash)
	entry, ok := db.ExecPayloads[key]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return entry, nil
}

func (db MockDB) GetRoBAnchorPayloadEntryBySlotPkHash(slot uint64, proposerPubkey, blockHash string, chainID string) (entry *ExecutionPayloadEntry, err error) {
	key := fmt.Sprintf("rob,%d-%s-%s-%s", slot, proposerPubkey, blockHash, chainID)
	entry, ok := db.ExecPayloads[key]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return entry, nil
}

func (db MockDB) GetExecutionPayloads(idFirst, idLast uint64) (entries []*ExecutionPayloadEntry, err error) {
	return nil, nil
}

func (db MockDB) DeleteExecutionPayloads(idFirst, idLast uint64) error {
	return nil
}

func (db MockDB) GetBlockSubmissionEntry(slot uint64, proposerPubkey, blockHash string) (entry *BuilderBlockSubmissionEntry, err error) {
	key := fmt.Sprintf("%d-%s-%s", slot, proposerPubkey, blockHash)
	entry, ok := db.BlockSubmissions[key]
	if !ok {
		return nil, fmt.Errorf(sql.ErrNoRows.Error())
	}

	return entry, nil
}

func (db MockDB) GetRecentDeliveredPayloads(filters GetPayloadsFilters) ([]*DeliveredPayloadEntry2, error) {
	return nil, nil
}

func (db MockDB) GetDeliveredPayloads(idFirst, idLast uint64) (entries []*DeliveredPayloadEntry2, err error) {
	return nil, nil
}

func (db MockDB) GetNumDeliveredPayloads() (uint64, error) {
	return 0, nil
}

func (db MockDB) GetBuilderSubmissions(filters GetBuilderSubmissionsFilters) ([]*BuilderBlockSubmissionEntry, error) {
	return nil, nil
}

func (db MockDB) GetBuilderSubmissionsBySlots(slotFrom, slotTo uint64) (entries []*BuilderBlockSubmissionEntry, err error) {
	return nil, nil
}

func (db MockDB) SaveDeliveredPayload(bidTrace *common.BidTraceV3, payloadResp *common.AnchorGetPayloadResponse, signedAt time.Time, publishMs uint64) error {
	return nil
}

func (db MockDB) UpsertBlockBuilderEntryAfterSubmission(lastSubmission *BuilderBlockSubmissionEntry, isToB bool, chainID string, isError bool) error {
	return nil
}

func (db MockDB) GetBlockBuilders() ([]*BlockBuilderEntry, error) {
	res := []*BlockBuilderEntry{}
	for _, v := range db.Builders {
		res = append(res, v)
	}
	return res, nil
}

func (db MockDB) GetBlockBuilderByPubkey(pubkey string) (*BlockBuilderEntry, error) {
	builder, ok := db.Builders[pubkey]
	if !ok {
		return nil, fmt.Errorf("builder with pubkey %v not in Builders map", pubkey) //nolint:goerr113
	}
	return builder, nil
}

func (db MockDB) SetBlockBuilderStatus(pubkey string, status common.BuilderStatus) error {
	builder, ok := db.Builders[pubkey]
	if !ok {
		return fmt.Errorf("builder with pubkey %v not in Builders map", pubkey) //nolint:goerr113
	}

	// Single key.
	builder.IsHighPrio = status.IsHighPrio
	builder.IsBlacklisted = status.IsBlacklisted
	builder.IsOptimistic = status.IsOptimistic
	return nil
}

func (db MockDB) SetBlockBuilderIDStatusIsOptimistic(pubkey string, isOptimistic bool) error {
	builder, ok := db.Builders[pubkey]
	if !ok {
		return fmt.Errorf("builder with pubkey %v not in Builders map", pubkey) //nolint:goerr113
	}
	for _, v := range db.Builders {
		if v.BuilderID == builder.BuilderID {
			v.IsOptimistic = isOptimistic
		}
	}
	return nil
}

func (db MockDB) SetBlockBuilderCollateral(pubkey, builderID, collateral string) error {
	builder, ok := db.Builders[pubkey]
	if !ok {
		return fmt.Errorf("builder with pubkey %v not in Builders map", pubkey) //nolint:goerr113
	}
	builder.BuilderID = builderID
	builder.Collateral = collateral
	return nil
}

func (db MockDB) IncBlockBuilderStatsAfterGetHeader(slot uint64, blockhash string) error {
	return nil
}

func (db MockDB) IncBlockBuilderStatsAfterGetPayload(builderPubkey string) error {
	return nil
}

// TODO: Add the below back when we handle builder demotion
/*
func (db MockDB) InsertBuilderDemotion(submitBlockRequest *common.BuilderSubmitBlockRequest, simError error) error {
	pubkey := submitBlockRequest.BuilderPubkey().String()
	db.Demotions[pubkey] = true
	return nil
}

func (db MockDB) UpdateBuilderDemotion(trace *common.BidTraceV2, signedBlock *common.SignedBeaconBlock, signedRegistration *types.SignedValidatorRegistration) error {
	pubkey := trace.BuilderPubkey.String()
	_, ok := db.Builders[pubkey]
	if !ok {
		return fmt.Errorf("builder with pubkey %v not in Builders map", pubkey) //nolint:goerr113
	}
	if !db.Demotions[pubkey] {
		return fmt.Errorf("builder with pubkey %v is not demoted", pubkey) //nolint:goerr113
	}
	db.Refunds[pubkey] = true
	return nil
}

func (db MockDB) GetBuilderDemotion(trace *common.BidTraceV2) (*BuilderDemotionEntry, error) {
	pubkey := trace.BuilderPubkey.String()
	_, ok := db.Builders[pubkey]
	if !ok {
		return nil, fmt.Errorf("builder with pubkey %v not in Builders map", pubkey) //nolint:goerr113
	}
	if db.Demotions[pubkey] {
		return &BuilderDemotionEntry{}, nil
	}
	return nil, nil
}
*/

func (db MockDB) GetTooLateGetPayload(slot uint64) (entries []*TooLateGetPayloadEntry, err error) {
	return nil, nil
}

func (db MockDB) InsertTooLateGetPayload(slot uint64, proposerPubkey, blockHash string, slotStart, requestTime, decodeTime, msIntoSlot uint64) error {
	return nil
}

func (db MockDB) InsertIncludedTobTx(txHash string, slot uint64, parentHash string, blockHash string) error {
	key := fmt.Sprintf("%d-%s-%s", slot, parentHash, blockHash)
	db.IncludedTobTxs[key] = append(db.IncludedTobTxs[key], &IncludedTobTxEntry{
		TxHash:     txHash,
		Slot:       slot,
		ParentHash: parentHash,
		BlockHash:  blockHash,
	})

	return nil
}

func (db MockDB) GetIncludedTobTxsForGivenSlotAndParentHashAndBlockHash(slot uint64, parentHash string, blockHash string) (entries []*IncludedTobTxEntry, err error) {
	key := fmt.Sprintf("%d-%s-%s", slot, parentHash, blockHash)
	entries, ok := db.IncludedTobTxs[key]
	if !ok {
		return nil, fmt.Errorf(sql.ErrNoRows.Error())
	}

	return entries, nil
}

func (db MockDB) InsertToBSubmitProfile(slot uint64, parentHash string, txHashes string, simulationDuration uint64, tracerDuration uint64, totalDuration uint64) error {
	key := fmt.Sprintf("tob,%d-%s-%s", slot, parentHash, txHashes)
	db.TobSubmitProfile[key] = &ToBSubmitProfileEntry{
		TxHashes:             txHashes,
		Slot:                 slot,
		ParentHash:           parentHash,
		SimulationDurationUs: simulationDuration,
		TracerDurationUs:     tracerDuration,
		TotalDurationUs:      totalDuration,
	}

	return nil
}

func (db MockDB) InsertRoBSubmitProfile(slot uint64, parentHash string, txHashes string, simulationDuration uint64, tracerDuration uint64, totalDuration uint64) error {
	key := fmt.Sprintf("rob,%d-%s-%s", slot, parentHash, txHashes)
	db.RobSubmitProfile[key] = &RoBSubmitProfileEntry{
		TxHashes:             txHashes,
		Slot:                 slot,
		ParentHash:           parentHash,
		SimulationDurationUs: simulationDuration,
		TracerDurationUs:     tracerDuration,
		TotalDurationUs:      totalDuration,
	}

	return nil
}

func (db MockDB) GetToBSubmitProfile(slot uint64, parentHash string, txHashes string) (entry *ToBSubmitProfileEntry, err error) {
	key := fmt.Sprintf("tob,%d-%s-%s", slot, parentHash, txHashes)

	res, ok := db.TobSubmitProfile[key]
	if !ok {
		return nil, fmt.Errorf(sql.ErrNoRows.Error())
	}

	return res, nil
}

func (db MockDB) GetRoBSubmitProfile(slot uint64, parentHash string, txHashes string) (entry *RoBSubmitProfileEntry, err error) {
	key := fmt.Sprintf("rob,%d-%s-%s", slot, parentHash, txHashes)

	res, ok := db.RobSubmitProfile[key]
	if !ok {
		return nil, fmt.Errorf(sql.ErrNoRows.Error())
	}

	return res, nil
}
