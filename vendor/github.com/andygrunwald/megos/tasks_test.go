package megos

import (
	"reflect"
	"testing"
)

func TestGetTaskByID_WithTask(t *testing.T) {
	taskID := "Task2"
	tasks := []Task{
		{ID: "Task1", Name: "Test task one"},
		{ID: "Task2", Name: "Test task two"},
		{ID: "Task3", Name: "Test task three"},
	}

	if ta, err := client.GetTaskByID(tasks, taskID); !reflect.DeepEqual(ta, &tasks[1]) {
		t.Errorf("Task is not the one as expected (%s). Expected %+v, got %+v", err, &tasks[1], ta)
	}
}

func TestGetTaskByID_WithoutTask(t *testing.T) {
	taskID := "Task4"
	tasks := []Task{
		{ID: "Task1", Name: "Test task one"},
		{ID: "Task2", Name: "Test task two"},
		{ID: "Task3", Name: "Test task three"},
	}

	ta, err := client.GetTaskByID(tasks, taskID)
	if ta != nil {
		t.Errorf("Task is not nil. Expected nil, got %+v", ta)
	}
	if err == nil {
		t.Errorf("err is nil. Expected a string, got %s", err)
	}
}
