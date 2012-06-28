package service

type Set struct {
	m map[string]int8
}

func NewSet() *Set {
	s := new(Set)
	s.m = make(map[string]int8)
	return s
}

func (s *Set) Add(item string) {
	s.m[item] = 1
}

func (s *Set) Remove(item string) {
	delete(s.m, item)
}

func (s *Set) Items() (items []string) {
	for k, _ := range s.m {
		items = append(items, k)
	}
	return
}
