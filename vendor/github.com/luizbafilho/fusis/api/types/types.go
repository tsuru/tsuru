package types

import (
	"errors"
	"fmt"
)

var (
	ErrServiceNotFound          error = ErrNotFound("service not found")
	ErrDestinationNotFound      error = ErrNotFound("destination not found")
	ErrServiceAlreadyExists           = errors.New("service already exists")
	ErrDestinationAlreadyExists       = errors.New("destination already exists")
)

type ErrNotFound string

func (e ErrNotFound) Error() string {
	return string(e)
}

type Service struct {
	Name         string `valid:"required"`
	Host         string
	Port         uint16 `valid:"required"`
	Protocol     string `valid:"required"`
	Scheduler    string `valid:"required"`
	Destinations []Destination
	Stats        *ServiceStats
}

type Destination struct {
	Name      string `valid:"required"`
	Host      string `valid:"required"`
	Port      uint16 `valid:"required"`
	Weight    int32
	Mode      string `valid:"required"`
	ServiceId string `valid:"required"`
	Stats     *DestinationStats
}

type ServiceStats struct {
	Connections uint32
	PacketsIn   uint32
	PacketsOut  uint32
	BytesIn     uint64
	BytesOut    uint64
	CPS         uint32
	PPSIn       uint32
	PPSOut      uint32
	BPSIn       uint32
	BPSOut      uint32
}

type DestinationStats struct {
	ActiveConns   uint32
	InactiveConns uint32
	PersistConns  uint32
}

func (svc Service) GetId() string {
	return svc.Name
}

func (dst Destination) GetId() string {
	return dst.Name
}

func (svc Service) KernelKey() string {
	return fmt.Sprintf("%s-%d-%s", svc.Host, svc.Port, svc.Protocol)
}

func (dst Destination) KernelKey() string {
	return fmt.Sprintf("%s-%d", dst.Host, dst.Port)
}
