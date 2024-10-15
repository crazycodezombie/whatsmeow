// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.35.1
// 	protoc        v3.21.12
// source: waUserPassword/WAProtobufsUserPassword.proto

package waUserPassword

import (
	reflect "reflect"
	sync "sync"

	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"

	_ "embed"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type UserPassword_Transformer int32

const (
	UserPassword_NONE               UserPassword_Transformer = 0
	UserPassword_PBKDF2_HMAC_SHA512 UserPassword_Transformer = 1
	UserPassword_PBKDF2_HMAC_SHA384 UserPassword_Transformer = 2
)

// Enum value maps for UserPassword_Transformer.
var (
	UserPassword_Transformer_name = map[int32]string{
		0: "NONE",
		1: "PBKDF2_HMAC_SHA512",
		2: "PBKDF2_HMAC_SHA384",
	}
	UserPassword_Transformer_value = map[string]int32{
		"NONE":               0,
		"PBKDF2_HMAC_SHA512": 1,
		"PBKDF2_HMAC_SHA384": 2,
	}
)

func (x UserPassword_Transformer) Enum() *UserPassword_Transformer {
	p := new(UserPassword_Transformer)
	*p = x
	return p
}

func (x UserPassword_Transformer) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (UserPassword_Transformer) Descriptor() protoreflect.EnumDescriptor {
	return file_waUserPassword_WAProtobufsUserPassword_proto_enumTypes[0].Descriptor()
}

func (UserPassword_Transformer) Type() protoreflect.EnumType {
	return &file_waUserPassword_WAProtobufsUserPassword_proto_enumTypes[0]
}

func (x UserPassword_Transformer) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Do not use.
func (x *UserPassword_Transformer) UnmarshalJSON(b []byte) error {
	num, err := protoimpl.X.UnmarshalJSONEnum(x.Descriptor(), b)
	if err != nil {
		return err
	}
	*x = UserPassword_Transformer(num)
	return nil
}

// Deprecated: Use UserPassword_Transformer.Descriptor instead.
func (UserPassword_Transformer) EnumDescriptor() ([]byte, []int) {
	return file_waUserPassword_WAProtobufsUserPassword_proto_rawDescGZIP(), []int{0, 0}
}

type UserPassword_Encoding int32

const (
	UserPassword_UTF8        UserPassword_Encoding = 0
	UserPassword_UTF8_BROKEN UserPassword_Encoding = 1
)

// Enum value maps for UserPassword_Encoding.
var (
	UserPassword_Encoding_name = map[int32]string{
		0: "UTF8",
		1: "UTF8_BROKEN",
	}
	UserPassword_Encoding_value = map[string]int32{
		"UTF8":        0,
		"UTF8_BROKEN": 1,
	}
)

func (x UserPassword_Encoding) Enum() *UserPassword_Encoding {
	p := new(UserPassword_Encoding)
	*p = x
	return p
}

func (x UserPassword_Encoding) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (UserPassword_Encoding) Descriptor() protoreflect.EnumDescriptor {
	return file_waUserPassword_WAProtobufsUserPassword_proto_enumTypes[1].Descriptor()
}

func (UserPassword_Encoding) Type() protoreflect.EnumType {
	return &file_waUserPassword_WAProtobufsUserPassword_proto_enumTypes[1]
}

func (x UserPassword_Encoding) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Do not use.
func (x *UserPassword_Encoding) UnmarshalJSON(b []byte) error {
	num, err := protoimpl.X.UnmarshalJSONEnum(x.Descriptor(), b)
	if err != nil {
		return err
	}
	*x = UserPassword_Encoding(num)
	return nil
}

// Deprecated: Use UserPassword_Encoding.Descriptor instead.
func (UserPassword_Encoding) EnumDescriptor() ([]byte, []int) {
	return file_waUserPassword_WAProtobufsUserPassword_proto_rawDescGZIP(), []int{0, 1}
}

type UserPassword struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Encoding        *UserPassword_Encoding         `protobuf:"varint,1,opt,name=encoding,enum=WAProtobufsUserPassword.UserPassword_Encoding" json:"encoding,omitempty"`
	Transformer     *UserPassword_Transformer      `protobuf:"varint,2,opt,name=transformer,enum=WAProtobufsUserPassword.UserPassword_Transformer" json:"transformer,omitempty"`
	TransformerArg  []*UserPassword_TransformerArg `protobuf:"bytes,3,rep,name=transformerArg" json:"transformerArg,omitempty"`
	TransformedData []byte                         `protobuf:"bytes,4,opt,name=transformedData" json:"transformedData,omitempty"`
}

func (x *UserPassword) Reset() {
	*x = UserPassword{}
	mi := &file_waUserPassword_WAProtobufsUserPassword_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *UserPassword) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*UserPassword) ProtoMessage() {}

func (x *UserPassword) ProtoReflect() protoreflect.Message {
	mi := &file_waUserPassword_WAProtobufsUserPassword_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use UserPassword.ProtoReflect.Descriptor instead.
func (*UserPassword) Descriptor() ([]byte, []int) {
	return file_waUserPassword_WAProtobufsUserPassword_proto_rawDescGZIP(), []int{0}
}

func (x *UserPassword) GetEncoding() UserPassword_Encoding {
	if x != nil && x.Encoding != nil {
		return *x.Encoding
	}
	return UserPassword_UTF8
}

func (x *UserPassword) GetTransformer() UserPassword_Transformer {
	if x != nil && x.Transformer != nil {
		return *x.Transformer
	}
	return UserPassword_NONE
}

func (x *UserPassword) GetTransformerArg() []*UserPassword_TransformerArg {
	if x != nil {
		return x.TransformerArg
	}
	return nil
}

func (x *UserPassword) GetTransformedData() []byte {
	if x != nil {
		return x.TransformedData
	}
	return nil
}

type UserPassword_TransformerArg struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Key   *string                            `protobuf:"bytes,1,opt,name=key" json:"key,omitempty"`
	Value *UserPassword_TransformerArg_Value `protobuf:"bytes,2,opt,name=value" json:"value,omitempty"`
}

func (x *UserPassword_TransformerArg) Reset() {
	*x = UserPassword_TransformerArg{}
	mi := &file_waUserPassword_WAProtobufsUserPassword_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *UserPassword_TransformerArg) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*UserPassword_TransformerArg) ProtoMessage() {}

func (x *UserPassword_TransformerArg) ProtoReflect() protoreflect.Message {
	mi := &file_waUserPassword_WAProtobufsUserPassword_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use UserPassword_TransformerArg.ProtoReflect.Descriptor instead.
func (*UserPassword_TransformerArg) Descriptor() ([]byte, []int) {
	return file_waUserPassword_WAProtobufsUserPassword_proto_rawDescGZIP(), []int{0, 0}
}

func (x *UserPassword_TransformerArg) GetKey() string {
	if x != nil && x.Key != nil {
		return *x.Key
	}
	return ""
}

func (x *UserPassword_TransformerArg) GetValue() *UserPassword_TransformerArg_Value {
	if x != nil {
		return x.Value
	}
	return nil
}

type UserPassword_TransformerArg_Value struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Types that are assignable to Value:
	//
	//	*UserPassword_TransformerArg_Value_AsBlob
	//	*UserPassword_TransformerArg_Value_AsUnsignedInteger
	Value isUserPassword_TransformerArg_Value_Value `protobuf_oneof:"value"`
}

func (x *UserPassword_TransformerArg_Value) Reset() {
	*x = UserPassword_TransformerArg_Value{}
	mi := &file_waUserPassword_WAProtobufsUserPassword_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *UserPassword_TransformerArg_Value) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*UserPassword_TransformerArg_Value) ProtoMessage() {}

func (x *UserPassword_TransformerArg_Value) ProtoReflect() protoreflect.Message {
	mi := &file_waUserPassword_WAProtobufsUserPassword_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use UserPassword_TransformerArg_Value.ProtoReflect.Descriptor instead.
func (*UserPassword_TransformerArg_Value) Descriptor() ([]byte, []int) {
	return file_waUserPassword_WAProtobufsUserPassword_proto_rawDescGZIP(), []int{0, 0, 0}
}

func (m *UserPassword_TransformerArg_Value) GetValue() isUserPassword_TransformerArg_Value_Value {
	if m != nil {
		return m.Value
	}
	return nil
}

func (x *UserPassword_TransformerArg_Value) GetAsBlob() []byte {
	if x, ok := x.GetValue().(*UserPassword_TransformerArg_Value_AsBlob); ok {
		return x.AsBlob
	}
	return nil
}

func (x *UserPassword_TransformerArg_Value) GetAsUnsignedInteger() uint32 {
	if x, ok := x.GetValue().(*UserPassword_TransformerArg_Value_AsUnsignedInteger); ok {
		return x.AsUnsignedInteger
	}
	return 0
}

type isUserPassword_TransformerArg_Value_Value interface {
	isUserPassword_TransformerArg_Value_Value()
}

type UserPassword_TransformerArg_Value_AsBlob struct {
	AsBlob []byte `protobuf:"bytes,1,opt,name=asBlob,oneof"`
}

type UserPassword_TransformerArg_Value_AsUnsignedInteger struct {
	AsUnsignedInteger uint32 `protobuf:"varint,2,opt,name=asUnsignedInteger,oneof"`
}

func (*UserPassword_TransformerArg_Value_AsBlob) isUserPassword_TransformerArg_Value_Value() {}

func (*UserPassword_TransformerArg_Value_AsUnsignedInteger) isUserPassword_TransformerArg_Value_Value() {
}

var File_waUserPassword_WAProtobufsUserPassword_proto protoreflect.FileDescriptor

//go:embed WAProtobufsUserPassword.pb.raw
var file_waUserPassword_WAProtobufsUserPassword_proto_rawDesc []byte

var (
	file_waUserPassword_WAProtobufsUserPassword_proto_rawDescOnce sync.Once
	file_waUserPassword_WAProtobufsUserPassword_proto_rawDescData = file_waUserPassword_WAProtobufsUserPassword_proto_rawDesc
)

func file_waUserPassword_WAProtobufsUserPassword_proto_rawDescGZIP() []byte {
	file_waUserPassword_WAProtobufsUserPassword_proto_rawDescOnce.Do(func() {
		file_waUserPassword_WAProtobufsUserPassword_proto_rawDescData = protoimpl.X.CompressGZIP(file_waUserPassword_WAProtobufsUserPassword_proto_rawDescData)
	})
	return file_waUserPassword_WAProtobufsUserPassword_proto_rawDescData
}

var file_waUserPassword_WAProtobufsUserPassword_proto_enumTypes = make([]protoimpl.EnumInfo, 2)
var file_waUserPassword_WAProtobufsUserPassword_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_waUserPassword_WAProtobufsUserPassword_proto_goTypes = []any{
	(UserPassword_Transformer)(0),             // 0: WAProtobufsUserPassword.UserPassword.Transformer
	(UserPassword_Encoding)(0),                // 1: WAProtobufsUserPassword.UserPassword.Encoding
	(*UserPassword)(nil),                      // 2: WAProtobufsUserPassword.UserPassword
	(*UserPassword_TransformerArg)(nil),       // 3: WAProtobufsUserPassword.UserPassword.TransformerArg
	(*UserPassword_TransformerArg_Value)(nil), // 4: WAProtobufsUserPassword.UserPassword.TransformerArg.Value
}
var file_waUserPassword_WAProtobufsUserPassword_proto_depIdxs = []int32{
	1, // 0: WAProtobufsUserPassword.UserPassword.encoding:type_name -> WAProtobufsUserPassword.UserPassword.Encoding
	0, // 1: WAProtobufsUserPassword.UserPassword.transformer:type_name -> WAProtobufsUserPassword.UserPassword.Transformer
	3, // 2: WAProtobufsUserPassword.UserPassword.transformerArg:type_name -> WAProtobufsUserPassword.UserPassword.TransformerArg
	4, // 3: WAProtobufsUserPassword.UserPassword.TransformerArg.value:type_name -> WAProtobufsUserPassword.UserPassword.TransformerArg.Value
	4, // [4:4] is the sub-list for method output_type
	4, // [4:4] is the sub-list for method input_type
	4, // [4:4] is the sub-list for extension type_name
	4, // [4:4] is the sub-list for extension extendee
	0, // [0:4] is the sub-list for field type_name
}

func init() { file_waUserPassword_WAProtobufsUserPassword_proto_init() }
func file_waUserPassword_WAProtobufsUserPassword_proto_init() {
	if File_waUserPassword_WAProtobufsUserPassword_proto != nil {
		return
	}
	file_waUserPassword_WAProtobufsUserPassword_proto_msgTypes[2].OneofWrappers = []any{
		(*UserPassword_TransformerArg_Value_AsBlob)(nil),
		(*UserPassword_TransformerArg_Value_AsUnsignedInteger)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_waUserPassword_WAProtobufsUserPassword_proto_rawDesc,
			NumEnums:      2,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_waUserPassword_WAProtobufsUserPassword_proto_goTypes,
		DependencyIndexes: file_waUserPassword_WAProtobufsUserPassword_proto_depIdxs,
		EnumInfos:         file_waUserPassword_WAProtobufsUserPassword_proto_enumTypes,
		MessageInfos:      file_waUserPassword_WAProtobufsUserPassword_proto_msgTypes,
	}.Build()
	File_waUserPassword_WAProtobufsUserPassword_proto = out.File
	file_waUserPassword_WAProtobufsUserPassword_proto_rawDesc = nil
	file_waUserPassword_WAProtobufsUserPassword_proto_goTypes = nil
	file_waUserPassword_WAProtobufsUserPassword_proto_depIdxs = nil
}
