// Code generated by MockGen. DO NOT EDIT.
// Source: factory.go

// Package mock is a generated GoMock package.
package mock

import (
	io "io"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	rx "github.com/yomorun/yomo/core/rx"
)

// MockFactory is a mock of Factory interface.
type MockFactory struct {
	ctrl     *gomock.Controller
	recorder *MockFactoryMockRecorder
}

// MockFactoryMockRecorder is the mock recorder for MockFactory.
type MockFactoryMockRecorder struct {
	mock *MockFactory
}

// NewMockFactory creates a new mock instance.
func NewMockFactory(ctrl *gomock.Controller) *MockFactory {
	mock := &MockFactory{ctrl: ctrl}
	mock.recorder = &MockFactoryMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockFactory) EXPECT() *MockFactoryMockRecorder {
	return m.recorder
}

// FromChannel mocks base method.
func (m *MockFactory) FromChannel(channel chan interface{}) rx.Stream {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FromChannel", channel)
	ret0, _ := ret[0].(rx.Stream)
	return ret0
}

// FromChannel indicates an expected call of FromChannel.
func (mr *MockFactoryMockRecorder) FromChannel(channel interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FromChannel", reflect.TypeOf((*MockFactory)(nil).FromChannel), channel)
}

// FromReader mocks base method.
func (m *MockFactory) FromReader(reader io.Reader) rx.Stream {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FromReader", reader)
	ret0, _ := ret[0].(rx.Stream)
	return ret0
}

// FromReader indicates an expected call of FromReader.
func (mr *MockFactoryMockRecorder) FromReader(reader interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FromReader", reflect.TypeOf((*MockFactory)(nil).FromReader), reader)
}

// FromReaderWithDecoder mocks base method.
func (m *MockFactory) FromReaderWithDecoder(readers chan io.Reader) rx.Stream {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FromReaderWithDecoder", readers)
	ret0, _ := ret[0].(rx.Stream)
	return ret0
}

// FromReaderWithDecoder indicates an expected call of FromReaderWithDecoder.
func (mr *MockFactoryMockRecorder) FromReaderWithDecoder(readers interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FromReaderWithDecoder", reflect.TypeOf((*MockFactory)(nil).FromReaderWithDecoder), readers)
}