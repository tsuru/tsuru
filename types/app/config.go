package app

type Config struct {
	Filename  string `json:"filename"`
	Content   string `json:"content"`
	NoRestart bool   `json:"noRestart"`
}
