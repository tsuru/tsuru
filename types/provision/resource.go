package provision

type ResourceGetter interface {
	GetMemory() int64
	GetMilliCPU() int
	GetPool() string
	GetCPUBurst() float64
}
