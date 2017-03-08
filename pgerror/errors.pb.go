// Code generated by protoc-gen-gogo.
// source: cockroach/pkg/sql/pgwire/pgerror/errors.proto
// DO NOT EDIT!

/*
	Package pgerror is a generated protocol buffer package.

	It is generated from these files:
		cockroach/pkg/sql/pgwire/pgerror/errors.proto

	It has these top-level messages:
		Error
*/
package pgerror

import proto "github.com/gogo/protobuf/proto"
import fmt "fmt"
import math "math"

import io "io"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion2 // please upgrade the proto package

// Error contains all Postgres wire protocol error fields.
// See https://www.postgresql.org/docs/current/static/protocol-error-fields.html
// for a list of all Postgres error fields, most of which are optional and can
// be used to provide auxiliary error information.
type Error struct {
	Code    string        `protobuf:"bytes,1,opt,name=code,proto3" json:"code,omitempty"`
	Message string        `protobuf:"bytes,2,opt,name=message,proto3" json:"message,omitempty"`
	Detail  string        `protobuf:"bytes,3,opt,name=detail,proto3" json:"detail,omitempty"`
	Hint    string        `protobuf:"bytes,4,opt,name=hint,proto3" json:"hint,omitempty"`
	Source  *Error_Source `protobuf:"bytes,5,opt,name=source" json:"source,omitempty"`
}

func (m *Error) Reset()                    { *m = Error{} }
func (m *Error) String() string            { return proto.CompactTextString(m) }
func (*Error) ProtoMessage()               {}
func (*Error) Descriptor() ([]byte, []int) { return fileDescriptorErrors, []int{0} }

type Error_Source struct {
	File     string `protobuf:"bytes,1,opt,name=file,proto3" json:"file,omitempty"`
	Line     int32  `protobuf:"varint,2,opt,name=line,proto3" json:"line,omitempty"`
	Function string `protobuf:"bytes,3,opt,name=function,proto3" json:"function,omitempty"`
}

func (m *Error_Source) Reset()                    { *m = Error_Source{} }
func (m *Error_Source) String() string            { return proto.CompactTextString(m) }
func (*Error_Source) ProtoMessage()               {}
func (*Error_Source) Descriptor() ([]byte, []int) { return fileDescriptorErrors, []int{0, 0} }

func init() {
	proto.RegisterType((*Error)(nil), "cockroach.pgerror.Error")
	proto.RegisterType((*Error_Source)(nil), "cockroach.pgerror.Error.Source")
}
func (m *Error) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Error) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if len(m.Code) > 0 {
		dAtA[i] = 0xa
		i++
		i = encodeVarintErrors(dAtA, i, uint64(len(m.Code)))
		i += copy(dAtA[i:], m.Code)
	}
	if len(m.Message) > 0 {
		dAtA[i] = 0x12
		i++
		i = encodeVarintErrors(dAtA, i, uint64(len(m.Message)))
		i += copy(dAtA[i:], m.Message)
	}
	if len(m.Detail) > 0 {
		dAtA[i] = 0x1a
		i++
		i = encodeVarintErrors(dAtA, i, uint64(len(m.Detail)))
		i += copy(dAtA[i:], m.Detail)
	}
	if len(m.Hint) > 0 {
		dAtA[i] = 0x22
		i++
		i = encodeVarintErrors(dAtA, i, uint64(len(m.Hint)))
		i += copy(dAtA[i:], m.Hint)
	}
	if m.Source != nil {
		dAtA[i] = 0x2a
		i++
		i = encodeVarintErrors(dAtA, i, uint64(m.Source.Size()))
		n1, err := m.Source.MarshalTo(dAtA[i:])
		if err != nil {
			return 0, err
		}
		i += n1
	}
	return i, nil
}

func (m *Error_Source) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Error_Source) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if len(m.File) > 0 {
		dAtA[i] = 0xa
		i++
		i = encodeVarintErrors(dAtA, i, uint64(len(m.File)))
		i += copy(dAtA[i:], m.File)
	}
	if m.Line != 0 {
		dAtA[i] = 0x10
		i++
		i = encodeVarintErrors(dAtA, i, uint64(m.Line))
	}
	if len(m.Function) > 0 {
		dAtA[i] = 0x1a
		i++
		i = encodeVarintErrors(dAtA, i, uint64(len(m.Function)))
		i += copy(dAtA[i:], m.Function)
	}
	return i, nil
}

func encodeFixed64Errors(dAtA []byte, offset int, v uint64) int {
	dAtA[offset] = uint8(v)
	dAtA[offset+1] = uint8(v >> 8)
	dAtA[offset+2] = uint8(v >> 16)
	dAtA[offset+3] = uint8(v >> 24)
	dAtA[offset+4] = uint8(v >> 32)
	dAtA[offset+5] = uint8(v >> 40)
	dAtA[offset+6] = uint8(v >> 48)
	dAtA[offset+7] = uint8(v >> 56)
	return offset + 8
}
func encodeFixed32Errors(dAtA []byte, offset int, v uint32) int {
	dAtA[offset] = uint8(v)
	dAtA[offset+1] = uint8(v >> 8)
	dAtA[offset+2] = uint8(v >> 16)
	dAtA[offset+3] = uint8(v >> 24)
	return offset + 4
}
func encodeVarintErrors(dAtA []byte, offset int, v uint64) int {
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return offset + 1
}
func (m *Error) Size() (n int) {
	var l int
	_ = l
	l = len(m.Code)
	if l > 0 {
		n += 1 + l + sovErrors(uint64(l))
	}
	l = len(m.Message)
	if l > 0 {
		n += 1 + l + sovErrors(uint64(l))
	}
	l = len(m.Detail)
	if l > 0 {
		n += 1 + l + sovErrors(uint64(l))
	}
	l = len(m.Hint)
	if l > 0 {
		n += 1 + l + sovErrors(uint64(l))
	}
	if m.Source != nil {
		l = m.Source.Size()
		n += 1 + l + sovErrors(uint64(l))
	}
	return n
}

func (m *Error_Source) Size() (n int) {
	var l int
	_ = l
	l = len(m.File)
	if l > 0 {
		n += 1 + l + sovErrors(uint64(l))
	}
	if m.Line != 0 {
		n += 1 + sovErrors(uint64(m.Line))
	}
	l = len(m.Function)
	if l > 0 {
		n += 1 + l + sovErrors(uint64(l))
	}
	return n
}

func sovErrors(x uint64) (n int) {
	for {
		n++
		x >>= 7
		if x == 0 {
			break
		}
	}
	return n
}
func sozErrors(x uint64) (n int) {
	return sovErrors(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *Error) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowErrors
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: Error: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Error: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Code", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowErrors
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= (uint64(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthErrors
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Code = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Message", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowErrors
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= (uint64(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthErrors
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Message = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 3:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Detail", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowErrors
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= (uint64(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthErrors
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Detail = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 4:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Hint", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowErrors
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= (uint64(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthErrors
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Hint = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 5:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Source", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowErrors
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthErrors
			}
			postIndex := iNdEx + msglen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Source == nil {
				m.Source = &Error_Source{}
			}
			if err := m.Source.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipErrors(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthErrors
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *Error_Source) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowErrors
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: Source: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Source: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field File", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowErrors
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= (uint64(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthErrors
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.File = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 2:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Line", wireType)
			}
			m.Line = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowErrors
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Line |= (int32(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 3:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Function", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowErrors
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= (uint64(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthErrors
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Function = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipErrors(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthErrors
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func skipErrors(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowErrors
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		wireType := int(wire & 0x7)
		switch wireType {
		case 0:
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowErrors
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if dAtA[iNdEx-1] < 0x80 {
					break
				}
			}
			return iNdEx, nil
		case 1:
			iNdEx += 8
			return iNdEx, nil
		case 2:
			var length int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowErrors
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				length |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			iNdEx += length
			if length < 0 {
				return 0, ErrInvalidLengthErrors
			}
			return iNdEx, nil
		case 3:
			for {
				var innerWire uint64
				var start int = iNdEx
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return 0, ErrIntOverflowErrors
					}
					if iNdEx >= l {
						return 0, io.ErrUnexpectedEOF
					}
					b := dAtA[iNdEx]
					iNdEx++
					innerWire |= (uint64(b) & 0x7F) << shift
					if b < 0x80 {
						break
					}
				}
				innerWireType := int(innerWire & 0x7)
				if innerWireType == 4 {
					break
				}
				next, err := skipErrors(dAtA[start:])
				if err != nil {
					return 0, err
				}
				iNdEx = start + next
			}
			return iNdEx, nil
		case 4:
			return iNdEx, nil
		case 5:
			iNdEx += 4
			return iNdEx, nil
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
	}
	panic("unreachable")
}

var (
	ErrInvalidLengthErrors = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowErrors   = fmt.Errorf("proto: integer overflow")
)

func init() { proto.RegisterFile("cockroach/pkg/sql/pgwire/pgerror/errors.proto", fileDescriptorErrors) }

var fileDescriptorErrors = []byte{
	// 253 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x64, 0x90, 0xb1, 0x4e, 0xc3, 0x30,
	0x18, 0x84, 0x6b, 0x68, 0x52, 0x30, 0x13, 0x1e, 0x90, 0xd5, 0xc1, 0x14, 0xa6, 0x2e, 0xd8, 0x12,
	0x0c, 0xec, 0x48, 0x6c, 0x4c, 0x61, 0x63, 0x0b, 0xae, 0x9b, 0x5a, 0x0d, 0xf9, 0x83, 0x9d, 0x8a,
	0xd7, 0xe0, 0xb1, 0x32, 0x32, 0x32, 0x82, 0x59, 0x78, 0x0c, 0xe4, 0x3f, 0x26, 0x4b, 0x17, 0xeb,
	0xee, 0xfe, 0xd3, 0xe9, 0x93, 0xe9, 0x95, 0x06, 0xbd, 0x75, 0x50, 0xea, 0x8d, 0x6a, 0xb7, 0x95,
	0xf2, 0xaf, 0xb5, 0x6a, 0xab, 0x37, 0xeb, 0x8c, 0x6a, 0x2b, 0xe3, 0x1c, 0x38, 0x85, 0xaf, 0x97,
	0xad, 0x83, 0x0e, 0xd8, 0xe9, 0x58, 0x97, 0xe9, 0x7e, 0xf9, 0x4b, 0x68, 0x76, 0x1f, 0x15, 0x63,
	0x74, 0xaa, 0x61, 0x65, 0x38, 0x59, 0x90, 0xe5, 0x71, 0x81, 0x9a, 0x71, 0x3a, 0x7b, 0x31, 0xde,
	0x97, 0x95, 0xe1, 0x07, 0x18, 0xff, 0x5b, 0x76, 0x46, 0xf3, 0x95, 0xe9, 0x4a, 0x5b, 0xf3, 0x43,
	0x3c, 0x24, 0x17, 0x57, 0x36, 0xb6, 0xe9, 0xf8, 0x74, 0x58, 0x89, 0x9a, 0xdd, 0xd2, 0xdc, 0xc3,
	0xce, 0x69, 0xc3, 0xb3, 0x05, 0x59, 0x9e, 0x5c, 0x9f, 0xcb, 0x3d, 0x0e, 0x89, 0x0c, 0xf2, 0x11,
	0x6b, 0x45, 0xaa, 0xcf, 0x1f, 0x68, 0x3e, 0x24, 0x71, 0x76, 0x6d, 0xeb, 0x11, 0x2e, 0xea, 0x98,
	0xd5, 0xb6, 0x19, 0xc8, 0xb2, 0x02, 0x35, 0x9b, 0xd3, 0xa3, 0xf5, 0xae, 0xd1, 0x9d, 0x85, 0x26,
	0x81, 0x8d, 0xfe, 0xee, 0xa2, 0xff, 0x16, 0x93, 0x3e, 0x08, 0xf2, 0x11, 0x04, 0xf9, 0x0c, 0x82,
	0x7c, 0x05, 0x41, 0xde, 0x7f, 0xc4, 0xe4, 0x69, 0x96, 0x28, 0x9e, 0x73, 0xfc, 0xa7, 0x9b, 0xbf,
	0x00, 0x00, 0x00, 0xff, 0xff, 0x49, 0xf0, 0x6b, 0xe3, 0x58, 0x01, 0x00, 0x00,
}
