package megos

import (
	"reflect"
	"testing"
)

func TestGetExecutorByID_WithExecutor(t *testing.T) {
	executorID := "Executor3"
	executor := []Executor{
		{ID: "Executor1", Name: "Test Executor one"},
		{ID: "Executor2", Name: "Test Executor two"},
		{ID: "Executor3", Name: "Test Executor three"},
	}

	if e, err := client.GetExecutorByID(executor, executorID); !reflect.DeepEqual(e, &executor[2]) {
		t.Errorf("Executor is not the one as expected (%s). Expected %+v, got %+v", err, &executor[2], e)
	}
}

func TestGetExecutorByID_WithoutExecutor(t *testing.T) {
	executorID := "Executor4"
	executor := []Executor{
		{ID: "Executor1", Name: "Test Executor one"},
		{ID: "Executor2", Name: "Test Executor two"},
		{ID: "Executor3", Name: "Test Executor three"},
	}

	e, err := client.GetExecutorByID(executor, executorID)
	if e != nil {
		t.Errorf("Executor is not nil. Expected nil, got %+v", e)
	}
	if err == nil {
		t.Errorf("err is nil. Expected a string, got %s", err)
	}
}
