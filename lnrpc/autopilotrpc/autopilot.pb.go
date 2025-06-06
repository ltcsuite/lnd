// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.30.0
// 	protoc        v3.12.4
// source: autopilotrpc/autopilot.proto

package autopilotrpc

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type StatusRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *StatusRequest) Reset() {
	*x = StatusRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_autopilotrpc_autopilot_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *StatusRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StatusRequest) ProtoMessage() {}

func (x *StatusRequest) ProtoReflect() protoreflect.Message {
	mi := &file_autopilotrpc_autopilot_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StatusRequest.ProtoReflect.Descriptor instead.
func (*StatusRequest) Descriptor() ([]byte, []int) {
	return file_autopilotrpc_autopilot_proto_rawDescGZIP(), []int{0}
}

type StatusResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Indicates whether the autopilot is active or not.
	Active bool `protobuf:"varint,1,opt,name=active,proto3" json:"active,omitempty"`
}

func (x *StatusResponse) Reset() {
	*x = StatusResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_autopilotrpc_autopilot_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *StatusResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StatusResponse) ProtoMessage() {}

func (x *StatusResponse) ProtoReflect() protoreflect.Message {
	mi := &file_autopilotrpc_autopilot_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StatusResponse.ProtoReflect.Descriptor instead.
func (*StatusResponse) Descriptor() ([]byte, []int) {
	return file_autopilotrpc_autopilot_proto_rawDescGZIP(), []int{1}
}

func (x *StatusResponse) GetActive() bool {
	if x != nil {
		return x.Active
	}
	return false
}

type ModifyStatusRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Whether the autopilot agent should be enabled or not.
	Enable bool `protobuf:"varint,1,opt,name=enable,proto3" json:"enable,omitempty"`
}

func (x *ModifyStatusRequest) Reset() {
	*x = ModifyStatusRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_autopilotrpc_autopilot_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ModifyStatusRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ModifyStatusRequest) ProtoMessage() {}

func (x *ModifyStatusRequest) ProtoReflect() protoreflect.Message {
	mi := &file_autopilotrpc_autopilot_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ModifyStatusRequest.ProtoReflect.Descriptor instead.
func (*ModifyStatusRequest) Descriptor() ([]byte, []int) {
	return file_autopilotrpc_autopilot_proto_rawDescGZIP(), []int{2}
}

func (x *ModifyStatusRequest) GetEnable() bool {
	if x != nil {
		return x.Enable
	}
	return false
}

type ModifyStatusResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *ModifyStatusResponse) Reset() {
	*x = ModifyStatusResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_autopilotrpc_autopilot_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ModifyStatusResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ModifyStatusResponse) ProtoMessage() {}

func (x *ModifyStatusResponse) ProtoReflect() protoreflect.Message {
	mi := &file_autopilotrpc_autopilot_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ModifyStatusResponse.ProtoReflect.Descriptor instead.
func (*ModifyStatusResponse) Descriptor() ([]byte, []int) {
	return file_autopilotrpc_autopilot_proto_rawDescGZIP(), []int{3}
}

type QueryScoresRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Pubkeys []string `protobuf:"bytes,1,rep,name=pubkeys,proto3" json:"pubkeys,omitempty"`
	// If set, we will ignore the local channel state when calculating scores.
	IgnoreLocalState bool `protobuf:"varint,2,opt,name=ignore_local_state,json=ignoreLocalState,proto3" json:"ignore_local_state,omitempty"`
}

func (x *QueryScoresRequest) Reset() {
	*x = QueryScoresRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_autopilotrpc_autopilot_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *QueryScoresRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*QueryScoresRequest) ProtoMessage() {}

func (x *QueryScoresRequest) ProtoReflect() protoreflect.Message {
	mi := &file_autopilotrpc_autopilot_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use QueryScoresRequest.ProtoReflect.Descriptor instead.
func (*QueryScoresRequest) Descriptor() ([]byte, []int) {
	return file_autopilotrpc_autopilot_proto_rawDescGZIP(), []int{4}
}

func (x *QueryScoresRequest) GetPubkeys() []string {
	if x != nil {
		return x.Pubkeys
	}
	return nil
}

func (x *QueryScoresRequest) GetIgnoreLocalState() bool {
	if x != nil {
		return x.IgnoreLocalState
	}
	return false
}

type QueryScoresResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Results []*QueryScoresResponse_HeuristicResult `protobuf:"bytes,1,rep,name=results,proto3" json:"results,omitempty"`
}

func (x *QueryScoresResponse) Reset() {
	*x = QueryScoresResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_autopilotrpc_autopilot_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *QueryScoresResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*QueryScoresResponse) ProtoMessage() {}

func (x *QueryScoresResponse) ProtoReflect() protoreflect.Message {
	mi := &file_autopilotrpc_autopilot_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use QueryScoresResponse.ProtoReflect.Descriptor instead.
func (*QueryScoresResponse) Descriptor() ([]byte, []int) {
	return file_autopilotrpc_autopilot_proto_rawDescGZIP(), []int{5}
}

func (x *QueryScoresResponse) GetResults() []*QueryScoresResponse_HeuristicResult {
	if x != nil {
		return x.Results
	}
	return nil
}

type SetScoresRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// The name of the heuristic to provide scores to.
	Heuristic string `protobuf:"bytes,1,opt,name=heuristic,proto3" json:"heuristic,omitempty"`
	// A map from hex-encoded public keys to scores. Scores must be in the range
	// [0.0, 1.0].
	Scores map[string]float64 `protobuf:"bytes,2,rep,name=scores,proto3" json:"scores,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"fixed64,2,opt,name=value,proto3"`
}

func (x *SetScoresRequest) Reset() {
	*x = SetScoresRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_autopilotrpc_autopilot_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SetScoresRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SetScoresRequest) ProtoMessage() {}

func (x *SetScoresRequest) ProtoReflect() protoreflect.Message {
	mi := &file_autopilotrpc_autopilot_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SetScoresRequest.ProtoReflect.Descriptor instead.
func (*SetScoresRequest) Descriptor() ([]byte, []int) {
	return file_autopilotrpc_autopilot_proto_rawDescGZIP(), []int{6}
}

func (x *SetScoresRequest) GetHeuristic() string {
	if x != nil {
		return x.Heuristic
	}
	return ""
}

func (x *SetScoresRequest) GetScores() map[string]float64 {
	if x != nil {
		return x.Scores
	}
	return nil
}

type SetScoresResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *SetScoresResponse) Reset() {
	*x = SetScoresResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_autopilotrpc_autopilot_proto_msgTypes[7]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SetScoresResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SetScoresResponse) ProtoMessage() {}

func (x *SetScoresResponse) ProtoReflect() protoreflect.Message {
	mi := &file_autopilotrpc_autopilot_proto_msgTypes[7]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SetScoresResponse.ProtoReflect.Descriptor instead.
func (*SetScoresResponse) Descriptor() ([]byte, []int) {
	return file_autopilotrpc_autopilot_proto_rawDescGZIP(), []int{7}
}

type QueryScoresResponse_HeuristicResult struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Heuristic string             `protobuf:"bytes,1,opt,name=heuristic,proto3" json:"heuristic,omitempty"`
	Scores    map[string]float64 `protobuf:"bytes,2,rep,name=scores,proto3" json:"scores,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"fixed64,2,opt,name=value,proto3"`
}

func (x *QueryScoresResponse_HeuristicResult) Reset() {
	*x = QueryScoresResponse_HeuristicResult{}
	if protoimpl.UnsafeEnabled {
		mi := &file_autopilotrpc_autopilot_proto_msgTypes[8]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *QueryScoresResponse_HeuristicResult) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*QueryScoresResponse_HeuristicResult) ProtoMessage() {}

func (x *QueryScoresResponse_HeuristicResult) ProtoReflect() protoreflect.Message {
	mi := &file_autopilotrpc_autopilot_proto_msgTypes[8]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use QueryScoresResponse_HeuristicResult.ProtoReflect.Descriptor instead.
func (*QueryScoresResponse_HeuristicResult) Descriptor() ([]byte, []int) {
	return file_autopilotrpc_autopilot_proto_rawDescGZIP(), []int{5, 0}
}

func (x *QueryScoresResponse_HeuristicResult) GetHeuristic() string {
	if x != nil {
		return x.Heuristic
	}
	return ""
}

func (x *QueryScoresResponse_HeuristicResult) GetScores() map[string]float64 {
	if x != nil {
		return x.Scores
	}
	return nil
}

var File_autopilotrpc_autopilot_proto protoreflect.FileDescriptor

var file_autopilotrpc_autopilot_proto_rawDesc = []byte{
	0x0a, 0x1c, 0x61, 0x75, 0x74, 0x6f, 0x70, 0x69, 0x6c, 0x6f, 0x74, 0x72, 0x70, 0x63, 0x2f, 0x61,
	0x75, 0x74, 0x6f, 0x70, 0x69, 0x6c, 0x6f, 0x74, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x0c,
	0x61, 0x75, 0x74, 0x6f, 0x70, 0x69, 0x6c, 0x6f, 0x74, 0x72, 0x70, 0x63, 0x22, 0x0f, 0x0a, 0x0d,
	0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x22, 0x28, 0x0a,
	0x0e, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12,
	0x16, 0x0a, 0x06, 0x61, 0x63, 0x74, 0x69, 0x76, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x08, 0x52,
	0x06, 0x61, 0x63, 0x74, 0x69, 0x76, 0x65, 0x22, 0x2d, 0x0a, 0x13, 0x4d, 0x6f, 0x64, 0x69, 0x66,
	0x79, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x16,
	0x0a, 0x06, 0x65, 0x6e, 0x61, 0x62, 0x6c, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x08, 0x52, 0x06,
	0x65, 0x6e, 0x61, 0x62, 0x6c, 0x65, 0x22, 0x16, 0x0a, 0x14, 0x4d, 0x6f, 0x64, 0x69, 0x66, 0x79,
	0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x5c,
	0x0a, 0x12, 0x51, 0x75, 0x65, 0x72, 0x79, 0x53, 0x63, 0x6f, 0x72, 0x65, 0x73, 0x52, 0x65, 0x71,
	0x75, 0x65, 0x73, 0x74, 0x12, 0x18, 0x0a, 0x07, 0x70, 0x75, 0x62, 0x6b, 0x65, 0x79, 0x73, 0x18,
	0x01, 0x20, 0x03, 0x28, 0x09, 0x52, 0x07, 0x70, 0x75, 0x62, 0x6b, 0x65, 0x79, 0x73, 0x12, 0x2c,
	0x0a, 0x12, 0x69, 0x67, 0x6e, 0x6f, 0x72, 0x65, 0x5f, 0x6c, 0x6f, 0x63, 0x61, 0x6c, 0x5f, 0x73,
	0x74, 0x61, 0x74, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x08, 0x52, 0x10, 0x69, 0x67, 0x6e, 0x6f,
	0x72, 0x65, 0x4c, 0x6f, 0x63, 0x61, 0x6c, 0x53, 0x74, 0x61, 0x74, 0x65, 0x22, 0xa6, 0x02, 0x0a,
	0x13, 0x51, 0x75, 0x65, 0x72, 0x79, 0x53, 0x63, 0x6f, 0x72, 0x65, 0x73, 0x52, 0x65, 0x73, 0x70,
	0x6f, 0x6e, 0x73, 0x65, 0x12, 0x4b, 0x0a, 0x07, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x73, 0x18,
	0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x31, 0x2e, 0x61, 0x75, 0x74, 0x6f, 0x70, 0x69, 0x6c, 0x6f,
	0x74, 0x72, 0x70, 0x63, 0x2e, 0x51, 0x75, 0x65, 0x72, 0x79, 0x53, 0x63, 0x6f, 0x72, 0x65, 0x73,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x2e, 0x48, 0x65, 0x75, 0x72, 0x69, 0x73, 0x74,
	0x69, 0x63, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x52, 0x07, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74,
	0x73, 0x1a, 0xc1, 0x01, 0x0a, 0x0f, 0x48, 0x65, 0x75, 0x72, 0x69, 0x73, 0x74, 0x69, 0x63, 0x52,
	0x65, 0x73, 0x75, 0x6c, 0x74, 0x12, 0x1c, 0x0a, 0x09, 0x68, 0x65, 0x75, 0x72, 0x69, 0x73, 0x74,
	0x69, 0x63, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x68, 0x65, 0x75, 0x72, 0x69, 0x73,
	0x74, 0x69, 0x63, 0x12, 0x55, 0x0a, 0x06, 0x73, 0x63, 0x6f, 0x72, 0x65, 0x73, 0x18, 0x02, 0x20,
	0x03, 0x28, 0x0b, 0x32, 0x3d, 0x2e, 0x61, 0x75, 0x74, 0x6f, 0x70, 0x69, 0x6c, 0x6f, 0x74, 0x72,
	0x70, 0x63, 0x2e, 0x51, 0x75, 0x65, 0x72, 0x79, 0x53, 0x63, 0x6f, 0x72, 0x65, 0x73, 0x52, 0x65,
	0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x2e, 0x48, 0x65, 0x75, 0x72, 0x69, 0x73, 0x74, 0x69, 0x63,
	0x52, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x2e, 0x53, 0x63, 0x6f, 0x72, 0x65, 0x73, 0x45, 0x6e, 0x74,
	0x72, 0x79, 0x52, 0x06, 0x73, 0x63, 0x6f, 0x72, 0x65, 0x73, 0x1a, 0x39, 0x0a, 0x0b, 0x53, 0x63,
	0x6f, 0x72, 0x65, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x14, 0x0a, 0x05, 0x76,
	0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x01, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75,
	0x65, 0x3a, 0x02, 0x38, 0x01, 0x22, 0xaf, 0x01, 0x0a, 0x10, 0x53, 0x65, 0x74, 0x53, 0x63, 0x6f,
	0x72, 0x65, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x1c, 0x0a, 0x09, 0x68, 0x65,
	0x75, 0x72, 0x69, 0x73, 0x74, 0x69, 0x63, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x68,
	0x65, 0x75, 0x72, 0x69, 0x73, 0x74, 0x69, 0x63, 0x12, 0x42, 0x0a, 0x06, 0x73, 0x63, 0x6f, 0x72,
	0x65, 0x73, 0x18, 0x02, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x2a, 0x2e, 0x61, 0x75, 0x74, 0x6f, 0x70,
	0x69, 0x6c, 0x6f, 0x74, 0x72, 0x70, 0x63, 0x2e, 0x53, 0x65, 0x74, 0x53, 0x63, 0x6f, 0x72, 0x65,
	0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x2e, 0x53, 0x63, 0x6f, 0x72, 0x65, 0x73, 0x45,
	0x6e, 0x74, 0x72, 0x79, 0x52, 0x06, 0x73, 0x63, 0x6f, 0x72, 0x65, 0x73, 0x1a, 0x39, 0x0a, 0x0b,
	0x53, 0x63, 0x6f, 0x72, 0x65, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x12, 0x10, 0x0a, 0x03, 0x6b,
	0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x14, 0x0a,
	0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x01, 0x52, 0x05, 0x76, 0x61,
	0x6c, 0x75, 0x65, 0x3a, 0x02, 0x38, 0x01, 0x22, 0x13, 0x0a, 0x11, 0x53, 0x65, 0x74, 0x53, 0x63,
	0x6f, 0x72, 0x65, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x32, 0xc9, 0x02, 0x0a,
	0x09, 0x41, 0x75, 0x74, 0x6f, 0x70, 0x69, 0x6c, 0x6f, 0x74, 0x12, 0x43, 0x0a, 0x06, 0x53, 0x74,
	0x61, 0x74, 0x75, 0x73, 0x12, 0x1b, 0x2e, 0x61, 0x75, 0x74, 0x6f, 0x70, 0x69, 0x6c, 0x6f, 0x74,
	0x72, 0x70, 0x63, 0x2e, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x1a, 0x1c, 0x2e, 0x61, 0x75, 0x74, 0x6f, 0x70, 0x69, 0x6c, 0x6f, 0x74, 0x72, 0x70, 0x63,
	0x2e, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12,
	0x55, 0x0a, 0x0c, 0x4d, 0x6f, 0x64, 0x69, 0x66, 0x79, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x12,
	0x21, 0x2e, 0x61, 0x75, 0x74, 0x6f, 0x70, 0x69, 0x6c, 0x6f, 0x74, 0x72, 0x70, 0x63, 0x2e, 0x4d,
	0x6f, 0x64, 0x69, 0x66, 0x79, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x1a, 0x22, 0x2e, 0x61, 0x75, 0x74, 0x6f, 0x70, 0x69, 0x6c, 0x6f, 0x74, 0x72, 0x70,
	0x63, 0x2e, 0x4d, 0x6f, 0x64, 0x69, 0x66, 0x79, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65,
	0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x52, 0x0a, 0x0b, 0x51, 0x75, 0x65, 0x72, 0x79, 0x53,
	0x63, 0x6f, 0x72, 0x65, 0x73, 0x12, 0x20, 0x2e, 0x61, 0x75, 0x74, 0x6f, 0x70, 0x69, 0x6c, 0x6f,
	0x74, 0x72, 0x70, 0x63, 0x2e, 0x51, 0x75, 0x65, 0x72, 0x79, 0x53, 0x63, 0x6f, 0x72, 0x65, 0x73,
	0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x21, 0x2e, 0x61, 0x75, 0x74, 0x6f, 0x70, 0x69,
	0x6c, 0x6f, 0x74, 0x72, 0x70, 0x63, 0x2e, 0x51, 0x75, 0x65, 0x72, 0x79, 0x53, 0x63, 0x6f, 0x72,
	0x65, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x4c, 0x0a, 0x09, 0x53, 0x65,
	0x74, 0x53, 0x63, 0x6f, 0x72, 0x65, 0x73, 0x12, 0x1e, 0x2e, 0x61, 0x75, 0x74, 0x6f, 0x70, 0x69,
	0x6c, 0x6f, 0x74, 0x72, 0x70, 0x63, 0x2e, 0x53, 0x65, 0x74, 0x53, 0x63, 0x6f, 0x72, 0x65, 0x73,
	0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x1f, 0x2e, 0x61, 0x75, 0x74, 0x6f, 0x70, 0x69,
	0x6c, 0x6f, 0x74, 0x72, 0x70, 0x63, 0x2e, 0x53, 0x65, 0x74, 0x53, 0x63, 0x6f, 0x72, 0x65, 0x73,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x42, 0x2c, 0x5a, 0x2a, 0x67, 0x69, 0x74, 0x68,
	0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x6c, 0x74, 0x63, 0x73, 0x75, 0x69, 0x74, 0x65, 0x2f,
	0x6c, 0x6e, 0x64, 0x2f, 0x6c, 0x6e, 0x72, 0x70, 0x63, 0x2f, 0x61, 0x75, 0x74, 0x6f, 0x70, 0x69,
	0x6c, 0x6f, 0x74, 0x72, 0x70, 0x63, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_autopilotrpc_autopilot_proto_rawDescOnce sync.Once
	file_autopilotrpc_autopilot_proto_rawDescData = file_autopilotrpc_autopilot_proto_rawDesc
)

func file_autopilotrpc_autopilot_proto_rawDescGZIP() []byte {
	file_autopilotrpc_autopilot_proto_rawDescOnce.Do(func() {
		file_autopilotrpc_autopilot_proto_rawDescData = protoimpl.X.CompressGZIP(file_autopilotrpc_autopilot_proto_rawDescData)
	})
	return file_autopilotrpc_autopilot_proto_rawDescData
}

var file_autopilotrpc_autopilot_proto_msgTypes = make([]protoimpl.MessageInfo, 11)
var file_autopilotrpc_autopilot_proto_goTypes = []interface{}{
	(*StatusRequest)(nil),                       // 0: autopilotrpc.StatusRequest
	(*StatusResponse)(nil),                      // 1: autopilotrpc.StatusResponse
	(*ModifyStatusRequest)(nil),                 // 2: autopilotrpc.ModifyStatusRequest
	(*ModifyStatusResponse)(nil),                // 3: autopilotrpc.ModifyStatusResponse
	(*QueryScoresRequest)(nil),                  // 4: autopilotrpc.QueryScoresRequest
	(*QueryScoresResponse)(nil),                 // 5: autopilotrpc.QueryScoresResponse
	(*SetScoresRequest)(nil),                    // 6: autopilotrpc.SetScoresRequest
	(*SetScoresResponse)(nil),                   // 7: autopilotrpc.SetScoresResponse
	(*QueryScoresResponse_HeuristicResult)(nil), // 8: autopilotrpc.QueryScoresResponse.HeuristicResult
	nil, // 9: autopilotrpc.QueryScoresResponse.HeuristicResult.ScoresEntry
	nil, // 10: autopilotrpc.SetScoresRequest.ScoresEntry
}
var file_autopilotrpc_autopilot_proto_depIdxs = []int32{
	8,  // 0: autopilotrpc.QueryScoresResponse.results:type_name -> autopilotrpc.QueryScoresResponse.HeuristicResult
	10, // 1: autopilotrpc.SetScoresRequest.scores:type_name -> autopilotrpc.SetScoresRequest.ScoresEntry
	9,  // 2: autopilotrpc.QueryScoresResponse.HeuristicResult.scores:type_name -> autopilotrpc.QueryScoresResponse.HeuristicResult.ScoresEntry
	0,  // 3: autopilotrpc.Autopilot.Status:input_type -> autopilotrpc.StatusRequest
	2,  // 4: autopilotrpc.Autopilot.ModifyStatus:input_type -> autopilotrpc.ModifyStatusRequest
	4,  // 5: autopilotrpc.Autopilot.QueryScores:input_type -> autopilotrpc.QueryScoresRequest
	6,  // 6: autopilotrpc.Autopilot.SetScores:input_type -> autopilotrpc.SetScoresRequest
	1,  // 7: autopilotrpc.Autopilot.Status:output_type -> autopilotrpc.StatusResponse
	3,  // 8: autopilotrpc.Autopilot.ModifyStatus:output_type -> autopilotrpc.ModifyStatusResponse
	5,  // 9: autopilotrpc.Autopilot.QueryScores:output_type -> autopilotrpc.QueryScoresResponse
	7,  // 10: autopilotrpc.Autopilot.SetScores:output_type -> autopilotrpc.SetScoresResponse
	7,  // [7:11] is the sub-list for method output_type
	3,  // [3:7] is the sub-list for method input_type
	3,  // [3:3] is the sub-list for extension type_name
	3,  // [3:3] is the sub-list for extension extendee
	0,  // [0:3] is the sub-list for field type_name
}

func init() { file_autopilotrpc_autopilot_proto_init() }
func file_autopilotrpc_autopilot_proto_init() {
	if File_autopilotrpc_autopilot_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_autopilotrpc_autopilot_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*StatusRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_autopilotrpc_autopilot_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*StatusResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_autopilotrpc_autopilot_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ModifyStatusRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_autopilotrpc_autopilot_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ModifyStatusResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_autopilotrpc_autopilot_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*QueryScoresRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_autopilotrpc_autopilot_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*QueryScoresResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_autopilotrpc_autopilot_proto_msgTypes[6].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SetScoresRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_autopilotrpc_autopilot_proto_msgTypes[7].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SetScoresResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_autopilotrpc_autopilot_proto_msgTypes[8].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*QueryScoresResponse_HeuristicResult); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_autopilotrpc_autopilot_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   11,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_autopilotrpc_autopilot_proto_goTypes,
		DependencyIndexes: file_autopilotrpc_autopilot_proto_depIdxs,
		MessageInfos:      file_autopilotrpc_autopilot_proto_msgTypes,
	}.Build()
	File_autopilotrpc_autopilot_proto = out.File
	file_autopilotrpc_autopilot_proto_rawDesc = nil
	file_autopilotrpc_autopilot_proto_goTypes = nil
	file_autopilotrpc_autopilot_proto_depIdxs = nil
}
