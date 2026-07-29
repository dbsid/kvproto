package metapb_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/pingcap/kvproto/pkg/metapb"
	"github.com/pingcap/kvproto/pkg/table_grouppb"
)

type legacyRegion struct {
	Id       uint64 `protobuf:"varint,1,opt,name=id,proto3" json:"id,omitempty"`
	StartKey []byte `protobuf:"bytes,2,opt,name=start_key,json=startKey,proto3" json:"start_key,omitempty"`
	EndKey   []byte `protobuf:"bytes,3,opt,name=end_key,json=endKey,proto3" json:"end_key,omitempty"`
}

func (m *legacyRegion) Reset()         { *m = legacyRegion{} }
func (m *legacyRegion) String() string { return proto.CompactTextString(m) }
func (*legacyRegion) ProtoMessage()    {}

func roundTrip(t *testing.T, input proto.Message, output proto.Message) {
	t.Helper()

	wire, err := proto.Marshal(input)
	if err != nil {
		t.Fatalf("marshal %T: %v", input, err)
	}
	if err := proto.Unmarshal(wire, output); err != nil {
		t.Fatalf("unmarshal %T: %v", output, err)
	}
	if !proto.Equal(input, output) {
		t.Fatalf("round trip changed message\ninput:  %s\noutput: %s", input, output)
	}
}

func testTableGroup() *table_grouppb.TableGroup {
	identity := &table_grouppb.TableGroupIdentity{
		KeyspaceId:   0,
		TableGroupId: 1001,
	}
	activeMembership := &table_grouppb.TableGroupMembership{
		Version: 2,
		Members: []*table_grouppb.TableGroupMember{
			{TableId: 10, IndexIds: []uint64{11, 12}},
			{TableId: 20, PartitionId: 21, IndexIds: []uint64{22}},
		},
	}
	preparedMembership := &table_grouppb.TableGroupMembership{
		Version: 3,
		Members: []*table_grouppb.TableGroupMember{
			{TableId: 10, IndexIds: []uint64{11, 12, 13}},
			{TableId: 20, PartitionId: 21, IndexIds: []uint64{22}},
		},
	}

	return &table_grouppb.TableGroup{
		Identity:           identity,
		MetadataVersion:    8,
		StatusVersion:      12,
		State:              table_grouppb.TableGroupState_TABLE_GROUP_STATE_UPDATING_MEMBERSHIP,
		ActiveMembership:   activeMembership,
		PreparedMembership: preparedMembership,
		PendingOperation: &table_grouppb.TableGroupOperation{
			Token:                 []byte("membership-operation"),
			Kind:                  table_grouppb.TableGroupOperationKind_TABLE_GROUP_OPERATION_KIND_UPDATE_MEMBERSHIP,
			BaseMetadataVersion:   7,
			TargetMetadataVersion: 8,
		},
		RegionBinding: &table_grouppb.TableGroupRegionBinding{
			RegionId:               2001,
			RegionEpoch:            &metapb.RegionEpoch{ConfVer: 4, Version: 5},
			ShardId:                3001,
			AppliedMetadataVersion: 7,
		},
		SplitPolicy: &table_grouppb.SplitPolicy{
			Mode: table_grouppb.SplitPolicyMode_SPLIT_POLICY_MODE_FORBID,
		},
		CapacityBudget: &table_grouppb.CapacityBudget{
			MaxDataBytes:           1 << 40,
			MaxWriteBytesPerSecond: 1 << 30,
			MaxRequestsPerSecond:   100000,
			MaxRaftLogBytes:        1 << 34,
			MaxSnapshotBytes:       1 << 39,
			MaxRecoverySeconds:     600,
			LimitAction:            table_grouppb.CapacityLimitAction_CAPACITY_LIMIT_ACTION_REJECT,
		},
		CapacityStatus: &table_grouppb.CapacityStatus{
			State: table_grouppb.CapacityState_CAPACITY_STATE_APPROACHING_LIMIT,
			Usage: &table_grouppb.CapacityUsage{
				DataBytes:           1 << 39,
				WriteBytesPerSecond: 1 << 29,
				RequestsPerSecond:   50000,
				RaftLogBytes:        1 << 33,
				SnapshotBytes:       1 << 38,
				RecoverySeconds:     300,
			},
			LimitingDimensions: []table_grouppb.CapacityDimension{
				table_grouppb.CapacityDimension_CAPACITY_DIMENSION_DATA_BYTES,
				table_grouppb.CapacityDimension_CAPACITY_DIMENSION_SNAPSHOT_BYTES,
			},
			ObservedAtUnixMillis: 1777777777000,
		},
		PlacementIntent: &table_grouppb.PlacementIntent{
			ReplicaCount:   3,
			LocationLabels: []string{"zone", "rack"},
			ReplicaConstraints: []*table_grouppb.PlacementConstraint{
				{
					Key:      "disk",
					Operator: table_grouppb.PlacementConstraintOperator_PLACEMENT_CONSTRAINT_OPERATOR_IN,
					Values:   []string{"nvme"},
				},
			},
			LeaderConstraints: []*table_grouppb.PlacementConstraint{
				{
					Key:      "engine",
					Operator: table_grouppb.PlacementConstraintOperator_PLACEMENT_CONSTRAINT_OPERATOR_NOT_IN,
					Values:   []string{"tiflash"},
				},
			},
		},
	}
}

func TestTableGroupMessagesRoundTrip(t *testing.T) {
	group := testTableGroup()

	tests := []struct {
		name   string
		input  proto.Message
		output proto.Message
	}{
		{
			name:   "table group snapshot",
			input:  group,
			output: &table_grouppb.TableGroup{},
		},
		{
			name: "create request",
			input: &table_grouppb.CreateTableGroupRequest{
				Header: &table_grouppb.RequestHeader{
					ClusterId:       42,
					SenderId:        43,
					CallerId:        "ddl-1",
					CallerComponent: "table-group-controller",
				},
				KeyspaceId:            0,
				RequestedTableGroupId: 1001,
				RegionBinding:         group.RegionBinding,
				SplitPolicy:           group.SplitPolicy,
				CapacityBudget:        group.CapacityBudget,
				PlacementIntent:       group.PlacementIntent,
				OperationToken:        []byte("create-operation"),
			},
			output: &table_grouppb.CreateTableGroupRequest{},
		},
		{
			name: "prepare request",
			input: &table_grouppb.PrepareMembershipRequest{
				Header:                  &table_grouppb.RequestHeader{ClusterId: 42},
				Identity:                group.Identity,
				ExpectedMetadataVersion: 7,
				ProposedMembership:      group.PreparedMembership,
				OperationToken:          group.PendingOperation.Token,
			},
			output: &table_grouppb.PrepareMembershipRequest{},
		},
		{
			name: "commit request",
			input: &table_grouppb.CommitMembershipRequest{
				Header:                  &table_grouppb.RequestHeader{ClusterId: 42},
				Identity:                group.Identity,
				ExpectedMetadataVersion: 8,
				MembershipVersion:       3,
				OperationToken:          group.PendingOperation.Token,
			},
			output: &table_grouppb.CommitMembershipRequest{},
		},
		{
			name: "abort request",
			input: &table_grouppb.AbortMembershipRequest{
				Header:                  &table_grouppb.RequestHeader{ClusterId: 42},
				Identity:                group.Identity,
				ExpectedMetadataVersion: 8,
				MembershipVersion:       3,
				OperationToken:          group.PendingOperation.Token,
			},
			output: &table_grouppb.AbortMembershipRequest{},
		},
		{
			name: "response error",
			input: &table_grouppb.GetTableGroupResponse{
				Header: &table_grouppb.ResponseHeader{
					ClusterId: 42,
					Error: &table_grouppb.TableGroupError{
						Code:                    table_grouppb.TableGroupErrorCode_TABLE_GROUP_ERROR_CODE_SPLIT_FORBIDDEN,
						Message:                 "table group region cannot split",
						Identity:                group.Identity,
						ExpectedMetadataVersion: 8,
						ActualMetadataVersion:   7,
						OperationToken:          group.PendingOperation.Token,
						SplitSource:             table_grouppb.SplitSource_SPLIT_SOURCE_AUTOMATIC_SIZE,
						SplitRejectionReason:    table_grouppb.SplitRejectionReason_SPLIT_REJECTION_REASON_POLICY_FORBIDS_SPLIT,
						CapacityDimensions: []table_grouppb.CapacityDimension{
							table_grouppb.CapacityDimension_CAPACITY_DIMENSION_DATA_BYTES,
						},
					},
				},
				TableGroup: group,
			},
			output: &table_grouppb.GetTableGroupResponse{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			roundTrip(t, test.input, test.output)
		})
	}
}

func TestOrdinaryRegionRoundTripDoesNotEnableTableGroup(t *testing.T) {
	ordinary := &metapb.Region{
		Id:       7,
		StartKey: []byte("a"),
		EndKey:   []byte("z"),
	}
	decoded := &metapb.Region{}
	roundTrip(t, ordinary, decoded)
	if decoded.TableGroup != nil {
		t.Fatalf("ordinary Region unexpectedly enabled Table Group: %v", decoded.TableGroup)
	}
}

func TestLegacyRegionReadsNewRegionWithoutEnablingPolicy(t *testing.T) {
	current := &metapb.Region{
		Id:       7,
		StartKey: []byte("a"),
		EndKey:   []byte("z"),
		TableGroup: &metapb.TableGroupRegionMeta{
			KeyspaceId:             0,
			TableGroupId:           1001,
			AppliedMetadataVersion: 8,
		},
	}
	wire, err := proto.Marshal(current)
	if err != nil {
		t.Fatalf("marshal current Region: %v", err)
	}

	legacy := &legacyRegion{}
	if err := proto.Unmarshal(wire, legacy); err != nil {
		t.Fatalf("legacy consumer failed to decode current Region: %v", err)
	}
	if legacy.Id != current.Id || string(legacy.StartKey) != "a" || string(legacy.EndKey) != "z" {
		t.Fatalf("legacy consumer changed existing fields: %+v", legacy)
	}

	legacyWire, err := proto.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy Region: %v", err)
	}
	decoded := &metapb.Region{}
	if err := proto.Unmarshal(legacyWire, decoded); err != nil {
		t.Fatalf("current consumer failed to decode legacy Region: %v", err)
	}
	if decoded.TableGroup != nil {
		t.Fatalf("legacy Region unexpectedly enabled Table Group: %v", decoded.TableGroup)
	}
}

func TestUnknownRegionFieldDoesNotEnableTableGroup(t *testing.T) {
	wire, err := proto.Marshal(&metapb.Region{Id: 7})
	if err != nil {
		t.Fatalf("marshal ordinary Region: %v", err)
	}
	// Field 100, wire type 2, with an opaque three-byte payload.
	wire = append(wire, 0xa2, 0x06, 0x03, 0x08, 0x01, 0x00)

	decoded := &metapb.Region{}
	if err := proto.Unmarshal(wire, decoded); err != nil {
		t.Fatalf("decode Region with unknown field: %v", err)
	}
	if decoded.TableGroup != nil {
		t.Fatalf("unknown field unexpectedly enabled Table Group: %v", decoded.TableGroup)
	}
}

func TestRegionTableGroupUsesFieldNumberNine(t *testing.T) {
	field, ok := reflect.TypeOf(metapb.Region{}).FieldByName("TableGroup")
	if !ok {
		t.Fatal("generated Region has no TableGroup field")
	}
	tag := field.Tag.Get("protobuf")
	if !strings.HasPrefix(tag, "bytes,9,") {
		t.Fatalf("Region.TableGroup protobuf tag = %q, want field number 9", tag)
	}
}
