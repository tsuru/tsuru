package megos

// State represents the JSON from the state.json of a mesos node
type State struct {
	Version                string      `json:"version"`
	GitSHA                 string      `json:"git_sha"`
	GitTag                 string      `json:"git_tag"`
	BuildDate              string      `json:"build_date"`
	BuildTime              float64     `json:"build_time"`
	BuildUser              string      `json:"build_user"`
	StartTime              float64     `json:"start_time"`
	ElectedTime            float64     `json:"elected_time"`
	ID                     string      `json:"id"`
	PID                    string      `json:"pid"`
	Hostname               string      `json:"hostname"`
	ActivatedSlaves        float64     `json:"activated_slaves"`
	DeactivatedSlaves      float64     `json:"deactivated_slaves"`
	Cluster                string      `json:"cluster"`
	Leader                 string      `json:"leader"`
	CompletedFrameworks    []Framework `json:"completed_frameworks"`
	OrphanTasks            []Task      `json:"orphan_tasks"`
	UnregisteredFrameworks []string    `json:"unregistered_frameworks"`
	Flags                  Flags       `json:"flags"`
	Slaves                 []Slave     `json:"slaves"`
	Frameworks             []Framework `json:"frameworks"`
	GitBranch              string      `json:"git_branch"`
	LogDir                 string      `json:"log_dir"`
	ExternalLogFile        string      `json:"external_log_file"`
}

// Flags represents the flags of a mesos state
type Flags struct {
	AppcStoreDir                     string `json:"appc_store_dir"`
	AllocationInterval               string `json:"allocation_interval"`
	Allocator                        string `json:"allocator"`
	Authenticate                     string `json:"authenticate"`
	AuthenticateHTTP                 string `json:"authenticate_http"`
	Authenticatee                    string `json:"authenticatee"`
	AuthenticateSlaves               string `json:"authenticate_slaves"`
	Authenticators                   string `json:"authenticators"`
	Authorizers                      string `json:"authorizers"`
	CgroupsCPUEnablePIDsAndTIDsCount string `json:"cgroups_cpu_enable_pids_and_tids_count"`
	CgroupsEnableCfs                 string `json:"cgroups_enable_cfs"`
	CgroupsHierarchy                 string `json:"cgroups_hierarchy"`
	CgroupsLimitSwap                 string `json:"cgroups_limit_swap"`
	CgroupsRoot                      string `json:"cgroups_root"`
	Cluster                          string `json:"cluster"`
	ContainerDiskWatchInterval       string `json:"container_disk_watch_interval"`
	Containerizers                   string `json:"containerizers"`
	DefaultRole                      string `json:"default_role"`
	DiskWatchInterval                string `json:"disk_watch_interval"`
	Docker                           string `json:"docker"`
	DockerKillOrphans                string `json:"docker_kill_orphans"`
	DockerRegistry                   string `json:"docker_registry"`
	DockerRemoveDelay                string `json:"docker_remove_delay"`
	DockerSandboxDirectory           string `json:"docker_sandbox_directory"`
	DockerSocket                     string `json:"docker_socket"`
	DockerStoreDir                   string `json:"docker_store_dir"`
	DockerStopTimeout                string `json:"docker_stop_timeout"`
	EnforceContainerDiskQuota        string `json:"enforce_container_disk_quota"`
	ExecutorRegistrationTimeout      string `json:"executor_registration_timeout"`
	ExecutorShutdownGracePeriod      string `json:"executor_shutdown_grace_period"`
	FetcherCacheDir                  string `json:"fetcher_cache_dir"`
	FetcherCacheSize                 string `json:"fetcher_cache_size"`
	FrameworksHome                   string `json:"frameworks_home"`
	FrameworkSorter                  string `json:"framework_sorter"`
	GCDelay                          string `json:"gc_delay"`
	GCDiskHeadroom                   string `json:"gc_disk_headroom"`
	HadoopHome                       string `json:"hadoop_home"`
	Help                             string `json:"help"`
	Hostname                         string `json:"hostname"`
	HostnameLookup                   string `json:"hostname_lookup"`
	HTTPAuthenticators               string `json:"http_authenticators"`
	ImageProvisionerBackend          string `json:"image_provisioner_backend"`
	InitializeDriverLogging          string `json:"initialize_driver_logging"`
	IP                               string `json:"ip"`
	Isolation                        string `json:"isolation"`
	LauncherDir                      string `json:"launcher_dir"`
	LogAutoInitialize                string `json:"log_auto_initialize"`
	LogDir                           string `json:"log_dir"`
	Logbufsecs                       string `json:"logbufsecs"`
	LoggingLevel                     string `json:"logging_level"`
	MaxCompletedFrameworks           string `json:"max_completed_frameworks"`
	MaxCompletedTasksPerFramework    string `json:"max_completed_tasks_per_framework"`
	MaxSlavePingTimeouts             string `json:"max_slave_ping_timeouts"`
	Master                           string `json:"master"`
	PerfDuration                     string `json:"perf_duration"`
	PerfInterval                     string `json:"perf_interval"`
	Port                             string `json:"port"`
	Quiet                            string `json:"quiet"`
	Quorum                           string `json:"quorum"`
	QOSCorrectionIntervalMin         string `json:"qos_correction_interval_min"`
	Recover                          string `json:"recover"`
	RevocableCPULowPriority          string `json:"revocable_cpu_low_priority"`
	RecoverySlaveRemovalLimit        string `json:"recovery_slave_removal_limit"`
	RecoveryTimeout                  string `json:"recovery_timeout"`
	RegistrationBackoffFactor        string `json:"registration_backoff_factor"`
	Registry                         string `json:"registry"`
	RegistryFetchTimeout             string `json:"registry_fetch_timeout"`
	RegistryStoreTimeout             string `json:"registry_store_timeout"`
	RegistryStrict                   string `json:"registry_strict"`
	ResourceMonitoringInterval       string `json:"resource_monitoring_interval"`
	RootSubmissions                  string `json:"root_submissions"`
	SandboxDirectory                 string `json:"sandbox_directory"`
	SlavePingTimeout                 string `json:"slave_ping_timeout"`
	SlaveReregisterTimeout           string `json:"slave_reregister_timeout"`
	Strict                           string `json:"strict"`
	SystemdRuntimeDirectory          string `json:"systemd_runtime_directory"`
	SwitchUser                       string `json:"switch_user"`
	OversubscribedResourcesInterval  string `json:"oversubscribed_resources_interval"`
	UserSorter                       string `json:"user_sorter"`
	Version                          string `json:"version"`
	WebuiDir                         string `json:"webui_dir"`
	WorkDir                          string `json:"work_dir"`
	ZK                               string `json:"zk"`
	ZKSessionTimeout                 string `json:"zk_session_timeout"`
}

// Framework represent a single framework of a mesos node
type Framework struct {
	Active             bool       `json:"active"`
	Checkpoint         bool       `json:"checkpoint"`
	CompletedTasks     []Task     `json:"completed_tasks"`
	Executors          []Executor `json:"executors"`
	CompletedExecutors []Executor `json:"completed_executors"`
	FailoverTimeout    float64    `json:"failover_timeout"`
	Hostname           string     `json:"hostname"`
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	PID                string     `json:"pid"`
	OfferedResources   Resources  `json:"offered_resources"`
	Offers             []Offer    `json:"offers"`
	RegisteredTime     float64    `json:"registered_time"`
	ReregisteredTime   float64    `json:"reregistered_time"`
	Resources          Resources  `json:"resources"`
	Role               string     `json:"role"`
	Tasks              []Task     `json:"tasks"`
	UnregisteredTime   float64    `json:"unregistered_time"`
	UsedResources      Resources  `json:"used_resources"`
	User               string     `json:"user"`
	WebuiURL           string     `json:"webui_url"`
	Labels             []Label    `json:"label"`
	// Missing fields
	// TODO: "capabilities": [],
}

// Offer represents a single offer from a Mesos Slave to a Mesos master
type Offer struct {
	ID          string            `json:"id"`
	FrameworkID string            `json:"framework_id"`
	SlaveID     string            `json:"slave_id"`
	Hostname    string            `json:"hostname"`
	URL         URL               `json:"url"`
	Resources   Resources         `json:"resources"`
	Attributes  map[string]string `json:"attributes"`
}

// URL represents a single URL
type URL struct {
	Scheme     string      `json:"scheme"`
	Address    Address     `json:"address"`
	Path       string      `json:"path"`
	Parameters []Parameter `json:"parameters"`
}

// Address represents a single address.
// e.g. from a Slave or from a Master
type Address struct {
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
	Port     int    `json:"port"`
}

// Parameter represents a single key / value pair for parameters
type Parameter struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Label represents a single key / value pair for labeling
type Label struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Task represent a single Mesos task
type Task struct {
	// Missing fields
	// TODO: "labels": [],
	ExecutorID  string        `json:"executor_id"`
	FrameworkID string        `json:"framework_id"`
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Resources   Resources     `json:"resources"`
	SlaveID     string        `json:"slave_id"`
	State       string        `json:"state"`
	Statuses    []TaskStatus  `json:"statuses"`
	Discovery   TaskDiscovery `json:"discovery"`
}

// TaskDiscovery represents the dicovery information of a task
type TaskDiscovery struct {
	Visibility string `json:"visibility"`
	Name       string `json:"name"`
	Ports      Ports  `json:"ports"`
}

// Ports represents a number of PortDetails
type Ports struct {
	Ports []PortDetails `json:"ports"`
}

// PortDetails represents details about a single port
type PortDetails struct {
	Number   int    `json:"number"`
	Protocol string `json:"protocol"`
}

// Resources represents a resource type for a task
type Resources struct {
	CPUs  float64 `json:"cpus"`
	Disk  float64 `json:"disk"`
	Mem   float64 `json:"mem"`
	Ports string  `json:"ports"`
}

// TaskStatus represents the status of a single task
type TaskStatus struct {
	State           string          `json:"state"`
	Timestamp       float64         `json:"timestamp"`
	ContainerStatus ContainerStatus `json:"container_status"`
}

// ContainerStatus represents the status of a single container inside a task
type ContainerStatus struct {
	NetworkInfos []NetworkInfo `json:"network_infos"`
}

// NetworkInfo represents information about the network of a container
type NetworkInfo struct {
	IpAddress   string      `json:"ip_address"`
	IpAddresses []IpAddress `json:"ip_addresses"`
}

// IpAddress represents a single IpAddress
type IpAddress struct {
	IpAddress string `json:"ip_address"`
}

// Slave represents a single mesos slave node
type Slave struct {
	Active              bool                   `json:"active"`
	Hostname            string                 `json:"hostname"`
	ID                  string                 `json:"id"`
	PID                 string                 `json:"pid"`
	RegisteredTime      float64                `json:"registered_time"`
	Resources           Resources              `json:"resources"`
	UsedResources       Resources              `json:"used_resources"`
	OfferedResources    Resources              `json:"offered_resources"`
	ReservedResources   Resources              `json:"reserved_resources"`
	UnreservedResources Resources              `json:"unreserved_resources"`
	Attributes          map[string]interface{} `json:"attributes"`
	Version             string                 `json:"version"`
}

// Executor represents a single executor of a framework
type Executor struct {
	CompletedTasks []Task    `json:"completed_tasks"`
	Container      string    `json:"container"`
	Directory      string    `json:"directory"`
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Resources      Resources `json:"resources"`
	Source         string    `json:"source"`
	QueuedTasks    []Task    `json:"queued_tasks"`
	Tasks          []Task    `json:"tasks"`
}

// System represents a system stats of a node
type System struct {
	AvgLoad15min  float64 `json:"avg_load_15min"`
	AvgLoad1min   float64 `json:"avg_load_1min"`
	AvgLoad5min   float64 `json:"avg_load_5min"`
	CpusTotal     float64 `json:"cpus_total"`
	MemFreeBytes  float64 `json:"mem_free_bytes"`
	MemTotalBytes float64 `json:"mem_total_bytes"`
}

// MetricsSnapshot represents the metrics of a node
type MetricsSnapshot struct {
	AllocatorEventQueueDispatches                          float64 `json:"allocator/event_queue_dispatches"`
	AllocatorMesosAllocationRunMs                          float64 `json:"allocator/mesos/allocation_run_ms"`
	AllocatorMesosAllocationRunMsCount                     float64 `json:"allocator/mesos/allocation_run_ms/count"`
	AllocatorMesosAllocationRunMsMax                       float64 `json:"allocator/mesos/allocation_run_ms/max"`
	AllocatorMesosAllocationRunMsMin                       float64 `json:"allocator/mesos/allocation_run_ms/min"`
	AllocatorMesosAllocationRunMsP50                       float64 `json:"allocator/mesos/allocation_run_ms/p50"`
	AllocatorMesosAllocationRunMsP90                       float64 `json:"allocator/mesos/allocation_run_ms/p90"`
	AllocatorMesosAllocationRunMsP95                       float64 `json:"allocator/mesos/allocation_run_ms/p95"`
	AllocatorMesosAllocationRunMsP99                       float64 `json:"allocator/mesos/allocation_run_ms/p99"`
	AllocatorMesosAllocationRunMsP999                      float64 `json:"allocator/mesos/allocation_run_ms/p999"`
	AllocatorMesosAllocationRunMsP9999                     float64 `json:"allocator/mesos/allocation_run_ms/p9999"`
	AllocatorMesosAllocationRuns                           float64 `json:"allocator/mesos/allocation_runs"`
	AllocatorMesosEventQueueDispatches                     float64 `json:"allocator/mesos/event_queue_dispatches"`
	AllocatorMesosOfferFiltersRolesActive                  float64 `json:"allocator/mesos/offer_filters/roles/*/active"`
	AllocatorMesosResourcesCpusOfferedorAllocated          float64 `json:"allocator/mesos/resources/cpus/offered_or_allocated"`
	AllocatorMesosResourcesCpusTotal                       float64 `json:"allocator/mesos/resources/cpus/total"`
	AllocatorMesosResourcesDiskOfferedorAllocated          float64 `json:"allocator/mesos/resources/disk/offered_or_allocated"`
	AllocatorMesosResourcesDiskTotal                       float64 `json:"allocator/mesos/resources/disk/total"`
	AllocatorMesosResourcesMemOfferedorAllocated           float64 `json:"allocator/mesos/resources/mem/offered_or_allocated"`
	AllocatorMesosResourcesMemTotal                        float64 `json:"allocator/mesos/resources/mem/total"`
	AllocatorMesosRolesSharesDominant                      float64 `json:"allocator/mesos/roles/*/shares/dominant"`
	MasterCpusPercent                                      float64 `json:"master/cpus_percent"`
	MasterCpusRevocablePercent                             float64 `json:"master/cpus_revocable_percent"`
	MasterCpusRevocableTotal                               float64 `json:"master/cpus_revocable_total"`
	MasterCpusRevocableUsed                                float64 `json:"master/cpus_revocable_used"`
	MasterCpusTotal                                        float64 `json:"master/cpus_total"`
	MasterCpusUsed                                         float64 `json:"master/cpus_used"`
	MasterDiskPercent                                      float64 `json:"master/disk_percent"`
	MasterDiskRevocablePercent                             float64 `json:"master/disk_revocable_percent"`
	MasterDiskRevocableTotal                               float64 `json:"master/disk_revocable_total"`
	MasterDiskRevocableUsed                                float64 `json:"master/disk_revocable_used"`
	MasterDiskTotal                                        float64 `json:"master/disk_total"`
	MasterDiskUsed                                         float64 `json:"master/disk_used"`
	MasterDroppedMessages                                  float64 `json:"master/dropped_messages"`
	MasterElected                                          float64 `json:"master/elected"`
	MasterEventQueueDispatches                             float64 `json:"master/event_queue_dispatches"`
	MasterEventQueueHttpRequests                           float64 `json:"master/event_queue_http_requests"`
	MasterEventQueueMessages                               float64 `json:"master/event_queue_messages"`
	MasterFrameworksActive                                 float64 `json:"master/frameworks_active"`
	MasterFrameworksConnected                              float64 `json:"master/frameworks_connected"`
	MasterFrameworksDisconnected                           float64 `json:"master/frameworks_disconnected"`
	MasterFrameworksInactive                               float64 `json:"master/frameworks_inactive"`
	MasterGpusPercent                                      float64 `json:"master/gpus_percent"`
	MasterGpusRevocablePercent                             float64 `json:"master/gpus_revocable_percent"`
	MasterGpusRevocableTotal                               float64 `json:"master/gpus_revocable_total"`
	MasterGpusRevocableUsed                                float64 `json:"master/gpus_revocable_used"`
	MasterGpusTotal                                        float64 `json:"master/gpus_total"`
	MasterGpusUsed                                         float64 `json:"master/gpus_used"`
	MasterInvalidExecutortoFrameworkMessages               float64 `json:"master/invalid_executor_to_framework_messages"`
	MasterInvalidFrameworktoExecutorMessages               float64 `json:"master/invalid_framework_to_executor_messages"`
	MasterInvalidStatusUpdateAcknowledgements              float64 `json:"master/invalid_status_update_acknowledgements"`
	MasterInvalidStatusUpdates                             float64 `json:"master/invalid_status_updates"`
	MasterMemPercent                                       float64 `json:"master/mem_percent"`
	MasterMemRevocablePercent                              float64 `json:"master/mem_revocable_percent"`
	MasterMemRevocableTotal                                float64 `json:"master/mem_revocable_total"`
	MasterMemRevocableUsed                                 float64 `json:"master/mem_revocable_used"`
	MasterMemTotal                                         float64 `json:"master/mem_total"`
	MasterMemUsed                                          float64 `json:"master/mem_used"`
	MasterMessagesAuthenticate                             float64 `json:"master/messages_authenticate"`
	MasterMessagesDeactivateFramework                      float64 `json:"master/messages_deactivate_framework"`
	MasterMessagesDeclineOffers                            float64 `json:"master/messages_decline_offers"`
	MasterMessagesExecutortoFramework                      float64 `json:"master/messages_executor_to_framework"`
	MasterMessagesExitedExecutor                           float64 `json:"master/messages_exited_executor"`
	MasterMessagesFrameworkToExecutor                      float64 `json:"master/messages_framework_to_executor"`
	MasterMessagesKillTask                                 float64 `json:"master/messages_kill_task"`
	MasterMessagesLaunchTasks                              float64 `json:"master/messages_launch_tasks"`
	MasterMessagesReconcileTasks                           float64 `json:"master/messages_reconcile_tasks"`
	MasterMessagesRegisterFramework                        float64 `json:"master/messages_register_framework"`
	MasterMessagesRegisterSlave                            float64 `json:"master/messages_register_slave"`
	MasterMessagesReregisterFramework                      float64 `json:"master/messages_reregister_framework"`
	MasterMessagesReregisterSlave                          float64 `json:"master/messages_reregister_slave"`
	MasterMessagesResourceRequest                          float64 `json:"master/messages_resource_request"`
	MasterMessagesReviveOffers                             float64 `json:"master/messages_revive_offers"`
	MasterMessagesStatusUpdate                             float64 `json:"master/messages_status_update"`
	MasterMessagesStatusUpdateAcknowledgement              float64 `json:"master/messages_status_update_acknowledgement"`
	MasterMessagesSuppressOffers                           float64 `json:"master/messages_suppress_offers"`
	MasterMessagesUnregisterFramework                      float64 `json:"master/messages_unregister_framework"`
	MasterMessagesUnregisterSlave                          float64 `json:"master/messages_unregister_slave"`
	MasterMessagesUpdateSlave                              float64 `json:"master/messages_update_slave"`
	MasterOutstandingOffers                                float64 `json:"master/outstanding_offers"`
	MasterRecoverySlaveRemovals                            float64 `json:"master/recovery_slave_removals"`
	MasterSlaveRegistrations                               float64 `json:"master/slave_registrations"`
	MasterSlaveRemovals                                    float64 `json:"master/slave_removals"`
	MasterSlaveRemovalsReasonRegistered                    float64 `json:"master/slave_removals/reason_registered"`
	MasterSlaveRemovalsReasonUnhealthy                     float64 `json:"master/slave_removals/reason_unhealthy"`
	MasterSlaveRemovalsReasonUnregistered                  float64 `json:"master/slave_removals/reason_unregistered"`
	MasterSlaveReregistrations                             float64 `json:"master/slave_reregistrations"`
	MasterSlaveShutdownsCanceled                           float64 `json:"master/slave_shutdowns_canceled"`
	MasterSlaveShutdownsCompleted                          float64 `json:"master/slave_shutdowns_completed"`
	MasterSlaveShutdownsScheduled                          float64 `json:"master/slave_shutdowns_scheduled"`
	MasterSlavesActive                                     float64 `json:"master/slaves_active"`
	MasterSlavesConnected                                  float64 `json:"master/slaves_connected"`
	MasterSlavesDisconnected                               float64 `json:"master/slaves_disconnected"`
	MasterSlavesInactive                                   float64 `json:"master/slaves_inactive"`
	MasterTaskFailedSourceSlaveReasonContainerLaunchFailed float64 `json:"master/task_failed/source_slave/reason_container_launch_failed"`
	MasterTaskKilledSourceSlaveReasonExecutorUnregistered  float64 `json:"master/task_killed/source_slave/reason_executor_unregistered"`
	MasterTasksError                                       float64 `json:"master/tasks_error"`
	MasterTasksFailed                                      float64 `json:"master/tasks_failed"`
	MasterTasksFinished                                    float64 `json:"master/tasks_finished"`
	MasterTasksKilled                                      float64 `json:"master/tasks_killed"`
	MasterTasksKilling                                     float64 `json:"master/tasks_killing"`
	MasterTasksLost                                        float64 `json:"master/tasks_lost"`
	MasterTasksRunning                                     float64 `json:"master/tasks_running"`
	MasterTasksStaging                                     float64 `json:"master/tasks_staging"`
	MasterTasksStarting                                    float64 `json:"master/tasks_starting"`
	MasterUptimeSecs                                       float64 `json:"master/uptime_secs"`
	MasterValidExecutortoFrameworkMessages                 float64 `json:"master/valid_executor_to_framework_messages"`
	MasterValidFrameworktoExecutorMessages                 float64 `json:"master/valid_framework_to_executor_messages"`
	MasterValidStatusUpdateAcknowledgements                float64 `json:"master/valid_status_update_acknowledgements"`
	MasterValidStatusUpdates                               float64 `json:"master/valid_status_updates"`
	RegistrarLogRecovered                                  float64 `json:"registrar/log/recovered"`
	RegistrarQueuedOperations                              float64 `json:"registrar/queued_operations"`
	RegistrarRegistrySizeBytes                             float64 `json:"registrar/registry_size_bytes"`
	RegistrarStateFetchMs                                  float64 `json:"registrar/state_fetch_ms"`
	RegistrarStateStoreMs                                  float64 `json:"registrar/state_store_ms"`
	RegistrarStateStoreMsCount                             float64 `json:"registrar/state_store_ms/count"`
	RegistrarStateStoreMsMax                               float64 `json:"registrar/state_store_ms/max"`
	RegistrarStateStoreMsMin                               float64 `json:"registrar/state_store_ms/min"`
	RegistrarStateStoreMsP50                               float64 `json:"registrar/state_store_ms/p50"`
	RegistrarStateStoreMsP90                               float64 `json:"registrar/state_store_ms/p90"`
	RegistrarStateStoreMsP95                               float64 `json:"registrar/state_store_ms/p95"`
	RegistrarStateStoreMsP99                               float64 `json:"registrar/state_store_ms/p99"`
	RegistrarStateStoreMsP999                              float64 `json:"registrar/state_store_ms/p999"`
	RegistrarStateStoreMsP9999                             float64 `json:"registrar/state_store_ms/p9999"`
	SystemCpusTotal                                        float64 `json:"system/cpus_total"`
	SystemLoad15min                                        float64 `json:"system/load_15min"`
	SystemLoad1min                                         float64 `json:"system/load_1min"`
	SystemLoad5min                                         float64 `json:"system/load_5min"`
	SystemMemFreeBytes                                     float64 `json:"system/mem_free_bytes"`
	SystemMemTotalBytes                                    float64 `json:"system/mem_total_bytes"`
}
