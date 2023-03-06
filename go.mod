module github.com/tsuru/tsuru

go 1.19

require (
	github.com/ajg/form v0.0.0-20160822230020-523a5da1a92f
	github.com/andrestc/docker-machine-driver-cloudstack v0.9.2
	github.com/aws/aws-sdk-go v1.28.2
	github.com/bradfitz/go-smtpd v0.0.0-20130623174436-5b56f4f917c7
	github.com/codegangsta/negroni v0.0.0-20140611175843-a13766a8c257
	github.com/diego-araujo/go-saml v0.0.0-20151211102911-81203d242537
	github.com/digitalocean/godo v0.0.0-20170404195252-dfa802149cae
	github.com/docker/cli v20.10.20+incompatible
	github.com/docker/docker v20.10.20+incompatible
	github.com/docker/machine v0.7.1-0.20190902101342-b170508bf44c
	github.com/elazarl/goproxy v0.0.0-20190711103511-473e67f1d7d2
	github.com/felixge/fgprof v0.9.1
	github.com/fsouza/go-dockerclient v1.7.4
	github.com/ghodss/yaml v1.0.0
	github.com/globalsign/mgo v0.0.0-20181015135952-eeefdecb41b8
	github.com/google/gops v0.0.0-20180311052415-160b358b10d6
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/websocket v1.4.2
	github.com/hashicorp/go-version v0.0.0-20180716215031-270f2f71b1ee
	github.com/kr/pretty v0.3.0
	github.com/mattn/go-shellwords v1.0.12
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d
	github.com/opentracing-contrib/go-stdlib v1.0.1-0.20201028152118-adbfc141dfc2
	github.com/opentracing/opentracing-go v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/pmorie/go-open-service-broker-client v0.0.0-20180330214919-dca737037ce6
	github.com/prometheus/client_golang v1.11.1
	github.com/prometheus/common v0.30.0
	github.com/sajari/fuzzy v1.0.0
	github.com/tsuru/commandmocker v0.0.0-20160909010208-e1d28f4f616a
	github.com/tsuru/config v0.0.0-20201023175036-375aaee8b560
	github.com/tsuru/deploy-agent v0.0.0-20221212211719-2bb1afb965c5
	github.com/tsuru/docker-cluster v0.0.0-20190325123005-f372d8d4e354
	github.com/tsuru/gnuflag v0.0.0-20151217162021-86b8c1b864aa
	github.com/tsuru/monsterqueue v0.0.0-20160909010522-70e946ec66c3
	github.com/tsuru/tablecli v0.0.0-20190131152944-7ded8a3383c6
	github.com/uber/jaeger-client-go v2.25.0+incompatible
	github.com/ugorji/go/codec v1.1.7
	golang.org/x/crypto v0.1.0
	golang.org/x/net v0.1.0
	golang.org/x/oauth2 v0.1.0
	golang.org/x/sys v0.1.0
	golang.org/x/term v0.1.0
	golang.org/x/text v0.4.0
	google.golang.org/grpc v1.47.0
	gopkg.in/amz.v3 v3.0.0-20161215130849-8c3190dff075
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gopkg.in/mgo.v2 v2.0.0-20180705113604-9856a29383ce
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.22.5
	k8s.io/apiextensions-apiserver v0.20.6
	k8s.io/apimachinery v0.22.5
	k8s.io/autoscaler/vertical-pod-autoscaler v0.9.2
	k8s.io/client-go v0.22.5
	k8s.io/code-generator v0.20.6
	k8s.io/ingress-gce v1.20.1
	k8s.io/metrics v0.20.6
)

require (
	cloud.google.com/go v0.97.0 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/Microsoft/go-winio v0.6.0 // indirect
	github.com/Microsoft/hcsshim v0.9.6 // indirect
	github.com/RobotsAndPencils/go-saml v0.0.0-20150922030833-aa127de49a01 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff v0.0.0-20160904140958-8edc80b07f38 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/codahale/hdrhistogram v0.0.0-20161010025455-3a0bb77429bd // indirect
	github.com/codegangsta/cli v1.19.1 // indirect
	github.com/containerd/cgroups v1.0.4 // indirect
	github.com/containerd/containerd v1.6.18 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/docker/docker-credential-helpers v0.7.0 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/elazarl/goproxy/ext v0.0.0-20190711103511-473e67f1d7d2 // indirect
	github.com/emicklei/go-restful v2.9.5+incompatible // indirect
	github.com/evanphx/json-patch v4.11.0+incompatible // indirect
	github.com/exoscale/egoscale v0.9.23 // indirect
	github.com/go-logr/logr v1.2.2 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.20.0 // indirect
	github.com/go-openapi/swag v0.19.5 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/go-cmp v0.5.8 // indirect
	github.com/google/go-querystring v0.0.0-20150414214848-547ef5ac9797 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/pprof v0.0.0-20210720184732-4bb14d4b1be1 // indirect
	github.com/googleapis/gax-go/v2 v2.1.1 // indirect
	github.com/googleapis/gnostic v0.5.5 // indirect
	github.com/howeyc/fsnotify v0.9.0 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/intel-go/cpuid v0.0.0-20181003105527-1a4a6f06a1c6 // indirect
	github.com/jinzhu/copier v0.0.0-20180308034124-7e38e58719c3 // indirect
	github.com/jmespath/go-jmespath v0.0.0-20180206201540-c2b33e8439af // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kardianos/osext v0.0.0-20151124170342-10da29423eb9 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/mailru/easyjson v0.7.0 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mitchellh/mapstructure v1.1.2 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/moby/sys/mount v0.2.0 // indirect
	github.com/moby/sys/mountinfo v0.5.0 // indirect
	github.com/moby/term v0.0.0-20221105221325-4eb28fa6025c // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0-rc2 // indirect
	github.com/opencontainers/runc v1.1.2 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/procfs v0.7.3 // indirect
	github.com/rackspace/gophercloud v0.0.0-20160825135439-c90cb954266e // indirect
	github.com/rogpeppe/go-internal v1.6.1 // indirect
	github.com/samalba/dockerclient v0.0.0-20160531175551-a30362618471 // indirect
	github.com/sirupsen/logrus v1.9.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/testify v1.8.1 // indirect
	github.com/tent/http-link-go v0.0.0-20130702225549-ac974c61c2f9 // indirect
	github.com/uber/jaeger-lib v2.2.0+incompatible // indirect
	github.com/vmware/govcloudair v0.0.2 // indirect
	github.com/xanzy/go-cloudstack/v2 v2.8.0 // indirect
	go.opencensus.io v0.23.0 // indirect
	go.uber.org/atomic v1.4.0 // indirect
	golang.org/x/mod v0.6.0 // indirect
	golang.org/x/time v0.0.0-20210723032227-1f47c861a9ac // indirect
	golang.org/x/tools v0.1.12 // indirect
	google.golang.org/api v0.60.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20220502173005-c8bf987b8c21 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/gengo v0.0.0-20201113003025-83324d819ded // indirect
	k8s.io/klog/v2 v2.50.2 // indirect
	k8s.io/kube-openapi v0.0.0-20211109043538-20434351676c // indirect
	k8s.io/utils v0.0.0-20220210201930-3a6ce19ff2f9 // indirect
	launchpad.net/gocheck v0.0.0-20140225173054-000000000087 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.1.2 // indirect
	sigs.k8s.io/yaml v1.2.0 // indirect
)

replace (
	github.com/ajg/form => github.com/cezarsa/form v0.0.0-20210510165411-863b166467b9
	github.com/samalba/dockerclient => github.com/cezarsa/dockerclient v0.0.0-20190924055524-af5052a88081
)
