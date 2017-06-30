package megos

import (
	"reflect"
	"testing"
)

func TestGetSlaveByID_WithSlave(t *testing.T) {
	slaveID := "Slave2"
	slaves := []Slave{
		{ID: "Slave1"},
		{ID: "Slave2"},
		{ID: "Slave3"},
	}

	if s, err := client.GetSlaveByID(slaves, slaveID); !reflect.DeepEqual(s, &slaves[1]) {
		t.Errorf("Slave is not the one as expected (%s). Expected %+v, got %+v", err, &slaves[1], s)
	}
}

func TestGetSlaveByID_WithoutSlave(t *testing.T) {
	slaveID := "Slave5"
	slaves := []Slave{
		{ID: "Slave1"},
		{ID: "Slave2"},
		{ID: "Slave3"},
	}

	s, err := client.GetSlaveByID(slaves, slaveID)
	if s != nil {
		t.Errorf("Slave is not nil. Expected nil, got %+v", s)
	}
	if err == nil {
		t.Errorf("err is nil. Expected a string, got %s", err)
	}
}
