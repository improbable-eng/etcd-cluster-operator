// Code generated by protoc-gen-go. DO NOT EDIT.
// source: proxy.proto

package proxy

import (
	context "context"
	fmt "fmt"
	proto "github.com/golang/protobuf/proto"
	timestamp "github.com/golang/protobuf/ptypes/timestamp"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

type UploadRequest struct {
	// The cluster identifier is a string that identifies this etcd cluster. It should be unique and stable between
	// backups and restores.
	ClusterIdentifier string `protobuf:"bytes,1,opt,name=cluster_identifier,json=clusterIdentifier,proto3" json:"cluster_identifier,omitempty"`
	// This is the time when the backup was actually made. This may well be different from the time it was uploaded.
	BackupMadeAt *timestamp.Timestamp `protobuf:"bytes,2,opt,name=backup_made_at,json=backupMadeAt,proto3" json:"backup_made_at,omitempty"`
	// This is the binary contents of the backup itself.
	Backup               []byte   `protobuf:"bytes,3,opt,name=backup,proto3" json:"backup,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *UploadRequest) Reset()         { *m = UploadRequest{} }
func (m *UploadRequest) String() string { return proto.CompactTextString(m) }
func (*UploadRequest) ProtoMessage()    {}
func (*UploadRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_700b50b08ed8dbaf, []int{0}
}

func (m *UploadRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_UploadRequest.Unmarshal(m, b)
}
func (m *UploadRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_UploadRequest.Marshal(b, m, deterministic)
}
func (m *UploadRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_UploadRequest.Merge(m, src)
}
func (m *UploadRequest) XXX_Size() int {
	return xxx_messageInfo_UploadRequest.Size(m)
}
func (m *UploadRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_UploadRequest.DiscardUnknown(m)
}

var xxx_messageInfo_UploadRequest proto.InternalMessageInfo

func (m *UploadRequest) GetClusterIdentifier() string {
	if m != nil {
		return m.ClusterIdentifier
	}
	return ""
}

func (m *UploadRequest) GetBackupMadeAt() *timestamp.Timestamp {
	if m != nil {
		return m.BackupMadeAt
	}
	return nil
}

func (m *UploadRequest) GetBackup() []byte {
	if m != nil {
		return m.Backup
	}
	return nil
}

type UploadReply struct {
	// This is the full URL of the backup in remote storage. The structure of this URL is entirely determined by the
	// proxy and may change from backup to backup, users should not attempt to parse data out of the URL.
	BackupUrl            string   `protobuf:"bytes,1,opt,name=backup_url,json=backupUrl,proto3" json:"backup_url,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *UploadReply) Reset()         { *m = UploadReply{} }
func (m *UploadReply) String() string { return proto.CompactTextString(m) }
func (*UploadReply) ProtoMessage()    {}
func (*UploadReply) Descriptor() ([]byte, []int) {
	return fileDescriptor_700b50b08ed8dbaf, []int{1}
}

func (m *UploadReply) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_UploadReply.Unmarshal(m, b)
}
func (m *UploadReply) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_UploadReply.Marshal(b, m, deterministic)
}
func (m *UploadReply) XXX_Merge(src proto.Message) {
	xxx_messageInfo_UploadReply.Merge(m, src)
}
func (m *UploadReply) XXX_Size() int {
	return xxx_messageInfo_UploadReply.Size(m)
}
func (m *UploadReply) XXX_DiscardUnknown() {
	xxx_messageInfo_UploadReply.DiscardUnknown(m)
}

var xxx_messageInfo_UploadReply proto.InternalMessageInfo

func (m *UploadReply) GetBackupUrl() string {
	if m != nil {
		return m.BackupUrl
	}
	return ""
}

type DownloadRequest struct {
	// This is the URL of the backup to download.
	BackupUrl            string   `protobuf:"bytes,2,opt,name=backup_url,json=backupUrl,proto3" json:"backup_url,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *DownloadRequest) Reset()         { *m = DownloadRequest{} }
func (m *DownloadRequest) String() string { return proto.CompactTextString(m) }
func (*DownloadRequest) ProtoMessage()    {}
func (*DownloadRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_700b50b08ed8dbaf, []int{2}
}

func (m *DownloadRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_DownloadRequest.Unmarshal(m, b)
}
func (m *DownloadRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_DownloadRequest.Marshal(b, m, deterministic)
}
func (m *DownloadRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_DownloadRequest.Merge(m, src)
}
func (m *DownloadRequest) XXX_Size() int {
	return xxx_messageInfo_DownloadRequest.Size(m)
}
func (m *DownloadRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_DownloadRequest.DiscardUnknown(m)
}

var xxx_messageInfo_DownloadRequest proto.InternalMessageInfo

func (m *DownloadRequest) GetBackupUrl() string {
	if m != nil {
		return m.BackupUrl
	}
	return ""
}

type DownloadReply struct {
	// This is the binary contents of the backup itself.
	Backup               []byte   `protobuf:"bytes,1,opt,name=backup,proto3" json:"backup,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *DownloadReply) Reset()         { *m = DownloadReply{} }
func (m *DownloadReply) String() string { return proto.CompactTextString(m) }
func (*DownloadReply) ProtoMessage()    {}
func (*DownloadReply) Descriptor() ([]byte, []int) {
	return fileDescriptor_700b50b08ed8dbaf, []int{3}
}

func (m *DownloadReply) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_DownloadReply.Unmarshal(m, b)
}
func (m *DownloadReply) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_DownloadReply.Marshal(b, m, deterministic)
}
func (m *DownloadReply) XXX_Merge(src proto.Message) {
	xxx_messageInfo_DownloadReply.Merge(m, src)
}
func (m *DownloadReply) XXX_Size() int {
	return xxx_messageInfo_DownloadReply.Size(m)
}
func (m *DownloadReply) XXX_DiscardUnknown() {
	xxx_messageInfo_DownloadReply.DiscardUnknown(m)
}

var xxx_messageInfo_DownloadReply proto.InternalMessageInfo

func (m *DownloadReply) GetBackup() []byte {
	if m != nil {
		return m.Backup
	}
	return nil
}

func init() {
	proto.RegisterType((*UploadRequest)(nil), "proxy.UploadRequest")
	proto.RegisterType((*UploadReply)(nil), "proxy.UploadReply")
	proto.RegisterType((*DownloadRequest)(nil), "proxy.DownloadRequest")
	proto.RegisterType((*DownloadReply)(nil), "proxy.DownloadReply")
}

func init() { proto.RegisterFile("proxy.proto", fileDescriptor_700b50b08ed8dbaf) }

var fileDescriptor_700b50b08ed8dbaf = []byte{
	// 283 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x64, 0x90, 0xc1, 0x4b, 0xc3, 0x30,
	0x18, 0xc5, 0xcd, 0x64, 0xc3, 0x7d, 0xdd, 0x14, 0x3f, 0xc6, 0x28, 0x05, 0xb1, 0xf4, 0x62, 0x0f,
	0xda, 0xc9, 0xf4, 0xe4, 0x49, 0xc1, 0x8b, 0x07, 0x41, 0x8a, 0x3b, 0x97, 0x74, 0xcd, 0x46, 0x31,
	0x5d, 0x62, 0x9a, 0xa2, 0xfd, 0x4f, 0xfc, 0x73, 0x65, 0x4d, 0xea, 0xb6, 0x7a, 0x7c, 0x2f, 0xdf,
	0xe3, 0xfd, 0xf2, 0xc0, 0x91, 0x4a, 0x7c, 0xd7, 0x91, 0x54, 0x42, 0x0b, 0xec, 0x37, 0xc2, 0xbb,
	0x5c, 0x0b, 0xb1, 0xe6, 0x6c, 0xd6, 0x98, 0x69, 0xb5, 0x9a, 0xe9, 0xbc, 0x60, 0xa5, 0xa6, 0x85,
	0x34, 0x77, 0xc1, 0x0f, 0x81, 0xf1, 0x42, 0x72, 0x41, 0xb3, 0x98, 0x7d, 0x56, 0xac, 0xd4, 0x78,
	0x03, 0xb8, 0xe4, 0x55, 0xa9, 0x99, 0x4a, 0xf2, 0x8c, 0x6d, 0x74, 0xbe, 0xca, 0x99, 0x72, 0x89,
	0x4f, 0xc2, 0x61, 0x7c, 0x6e, 0x5f, 0x5e, 0xfe, 0x1e, 0xf0, 0x11, 0x4e, 0x53, 0xba, 0xfc, 0xa8,
	0x64, 0x52, 0xd0, 0x8c, 0x25, 0x54, 0xbb, 0x3d, 0x9f, 0x84, 0xce, 0xdc, 0x8b, 0x4c, 0x75, 0xd4,
	0x56, 0x47, 0xef, 0x6d, 0x75, 0x3c, 0x32, 0x89, 0x57, 0x9a, 0xb1, 0x27, 0x8d, 0x53, 0x18, 0x18,
	0xed, 0x1e, 0xfb, 0x24, 0x1c, 0xc5, 0x56, 0x05, 0xd7, 0xe0, 0xb4, 0x64, 0x92, 0xd7, 0x78, 0x01,
	0x60, 0x8b, 0x2a, 0xc5, 0x2d, 0xcf, 0xd0, 0x38, 0x0b, 0xc5, 0x83, 0x5b, 0x38, 0x7b, 0x16, 0x5f,
	0x9b, 0xfd, 0x9f, 0x1c, 0x26, 0x7a, 0xdd, 0xc4, 0x15, 0x8c, 0x77, 0x89, 0x6d, 0xc3, 0x0e, 0x84,
	0xec, 0x83, 0xcc, 0x6b, 0xe8, 0xbf, 0x6d, 0xd7, 0xc4, 0x7b, 0x18, 0x18, 0x22, 0x9c, 0x44, 0x66,
	0xec, 0x83, 0xe9, 0x3c, 0xec, 0xb8, 0x92, 0xd7, 0xc1, 0x11, 0x3e, 0xc0, 0x49, 0xdb, 0x83, 0x53,
	0x7b, 0xd1, 0x41, 0xf5, 0x26, 0xff, 0xfc, 0x26, 0x9b, 0x0e, 0x9a, 0xf5, 0xee, 0x7e, 0x03, 0x00,
	0x00, 0xff, 0xff, 0xa7, 0xdb, 0x8e, 0x4d, 0xdc, 0x01, 0x00, 0x00,
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// ProxyClient is the client API for Proxy service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type ProxyClient interface {
	// Upload will store a backup file in cloud storage
	Upload(ctx context.Context, in *UploadRequest, opts ...grpc.CallOption) (*UploadReply, error)
	// Download will retrieve a backup file from cloud storage
	Download(ctx context.Context, in *DownloadRequest, opts ...grpc.CallOption) (*DownloadReply, error)
}

type proxyClient struct {
	cc *grpc.ClientConn
}

func NewProxyClient(cc *grpc.ClientConn) ProxyClient {
	return &proxyClient{cc}
}

func (c *proxyClient) Upload(ctx context.Context, in *UploadRequest, opts ...grpc.CallOption) (*UploadReply, error) {
	out := new(UploadReply)
	err := c.cc.Invoke(ctx, "/proxy.Proxy/Upload", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *proxyClient) Download(ctx context.Context, in *DownloadRequest, opts ...grpc.CallOption) (*DownloadReply, error) {
	out := new(DownloadReply)
	err := c.cc.Invoke(ctx, "/proxy.Proxy/Download", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ProxyServer is the server API for Proxy service.
type ProxyServer interface {
	// Upload will store a backup file in cloud storage
	Upload(context.Context, *UploadRequest) (*UploadReply, error)
	// Download will retrieve a backup file from cloud storage
	Download(context.Context, *DownloadRequest) (*DownloadReply, error)
}

// UnimplementedProxyServer can be embedded to have forward compatible implementations.
type UnimplementedProxyServer struct {
}

func (*UnimplementedProxyServer) Upload(ctx context.Context, req *UploadRequest) (*UploadReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Upload not implemented")
}
func (*UnimplementedProxyServer) Download(ctx context.Context, req *DownloadRequest) (*DownloadReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Download not implemented")
}

func RegisterProxyServer(s *grpc.Server, srv ProxyServer) {
	s.RegisterService(&_Proxy_serviceDesc, srv)
}

func _Proxy_Upload_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UploadRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ProxyServer).Upload(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/proxy.Proxy/Upload",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ProxyServer).Upload(ctx, req.(*UploadRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Proxy_Download_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DownloadRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ProxyServer).Download(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/proxy.Proxy/Download",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ProxyServer).Download(ctx, req.(*DownloadRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _Proxy_serviceDesc = grpc.ServiceDesc{
	ServiceName: "proxy.Proxy",
	HandlerType: (*ProxyServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Upload",
			Handler:    _Proxy_Upload_Handler,
		},
		{
			MethodName: "Download",
			Handler:    _Proxy_Download_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "proxy.proto",
}
