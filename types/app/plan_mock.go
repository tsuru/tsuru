package app

var _ PlanStorage = &MockPlanStorage{}

// MockPlanStorage implements PlanStorage interface
type MockPlanStorage struct {
	OnInsert      func(Plan) error
	OnFindAll     func() ([]Plan, error)
	OnFindDefault func() (*Plan, error)
	OnFindByName  func(string) (*Plan, error)
	OnDelete      func(Plan) error
}

func (m *MockPlanStorage) Insert(p Plan) error {
	return m.OnInsert(p)
}

func (m *MockPlanStorage) FindAll() ([]Plan, error) {
	return m.OnFindAll()
}

func (m *MockPlanStorage) FindDefault() (*Plan, error) {
	return m.OnFindDefault()
}

func (m *MockPlanStorage) FindByName(name string) (*Plan, error) {
	return m.OnFindByName(name)
}

func (m *MockPlanStorage) Delete(p Plan) error {
	return m.OnDelete(p)
}

// MockPlanService implements PlanService interface
type MockPlanService struct {
	OnCreate      func(Plan) error
	OnList        func() ([]Plan, error)
	OnFindByName  func(string) (*Plan, error)
	OnDefaultPlan func() (*Plan, error)
	OnRemove      func(string) error
}

func (m *MockPlanService) Create(plan Plan) error {
	if m.OnCreate == nil {
		return nil
	}
	return m.OnCreate(plan)
}

func (m *MockPlanService) List() ([]Plan, error) {
	if m.OnList == nil {
		return nil, nil
	}
	return m.OnList()
}

func (m *MockPlanService) FindByName(name string) (*Plan, error) {
	if m.OnFindByName == nil {
		return nil, nil
	}
	return m.OnFindByName(name)
}

func (m *MockPlanService) DefaultPlan() (*Plan, error) {
	if m.OnDefaultPlan == nil {
		return nil, nil
	}
	return m.OnDefaultPlan()
}

func (m *MockPlanService) Remove(name string) error {
	if m.OnRemove == nil {
		return nil
	}
	return m.OnRemove(name)
}
