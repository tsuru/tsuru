module github.com/tsuru/tsuru

go 1.12

require (
	github.com/Microsoft/go-winio v0.4.14 // indirect
	github.com/Nvveen/Gotty v0.0.0-20120604004816-cd527374f1e5 // indirect
	github.com/RobotsAndPencils/go-saml v0.0.0-20150922030833-aa127de49a01 // indirect
	github.com/ajg/form v0.0.0-20160822230020-523a5da1a92f
	github.com/andrestc/docker-machine-driver-cloudstack v0.9.2
	github.com/armon/go-proxyproto v0.0.0-20190211145416-68259f75880e // indirect
	github.com/aws/aws-sdk-go v1.16.21
	github.com/bradfitz/go-smtpd v0.0.0-20130623174436-5b56f4f917c7
	github.com/cenkalti/backoff v0.0.0-20160904140958-8edc80b07f38 // indirect
	github.com/codegangsta/cli v1.19.1 // indirect
	github.com/codegangsta/negroni v0.0.0-20140611175843-a13766a8c257
	github.com/containerd/continuity v0.0.0-20190426062206-aaeac12a7ffc // indirect
	github.com/coreos/etcd v3.3.25+incompatible // indirect
	github.com/diego-araujo/go-saml v0.0.0-20151211102911-81203d242537
	github.com/digitalocean/godo v0.0.0-20170404195252-dfa802149cae
	github.com/docker/docker v20.10.2+incompatible
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/libnetwork v0.8.0-dev.2.0.20180706232811-d00ceed44cc4 // indirect
	github.com/docker/machine v0.7.1-0.20190902101342-b170508bf44c
	github.com/elazarl/goproxy v0.0.0-20190711103511-473e67f1d7d2 // indirect
	github.com/elazarl/goproxy/ext v0.0.0-20190711103511-473e67f1d7d2 // indirect
	github.com/exoscale/egoscale v0.9.23 // indirect
	github.com/fatih/structs v1.1.0 // indirect
	github.com/felixge/fgprof v0.9.1
	github.com/fsouza/go-dockerclient v0.0.0-20180427001620-3a206030a28a
	github.com/garyburd/redigo v1.6.0 // indirect
	github.com/ghodss/yaml v1.0.0
	github.com/globalsign/mgo v0.0.0-20181015135952-eeefdecb41b8
	github.com/google/go-cmp v0.4.0 // indirect
	github.com/google/go-querystring v0.0.0-20150414214848-547ef5ac9797 // indirect
	github.com/google/gops v0.0.0-20180311052415-160b358b10d6
	github.com/gorilla/mux v1.7.0
	github.com/gorilla/pat v0.0.0-20131205071617-ae2e162c4b2a // indirect
	github.com/gorilla/websocket v1.4.1
	github.com/hashicorp/go-version v0.0.0-20180716215031-270f2f71b1ee
	github.com/intel-go/cpuid v0.0.0-20181003105527-1a4a6f06a1c6 // indirect
	github.com/jinzhu/copier v0.0.0-20180308034124-7e38e58719c3 // indirect
	github.com/kardianos/osext v0.0.0-20151124170342-10da29423eb9 // indirect
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/kr/pretty v0.2.0
	github.com/mailgun/holster v3.0.0+incompatible // indirect
	github.com/mailgun/metrics v0.0.0-20170714162148-fd99b46995bd // indirect
	github.com/mattn/go-shellwords v1.0.2
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/opencontainers/runc v0.1.1 // indirect
	github.com/opentracing-contrib/go-stdlib v1.0.1-0.20201028152118-adbfc141dfc2
	github.com/opentracing/opentracing-go v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/pmorie/go-open-service-broker-client v0.0.0-20180330214919-dca737037ce6
	github.com/prometheus/client_golang v1.1.0
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.6.0
	github.com/rackspace/gophercloud v0.0.0-20160825135439-c90cb954266e // indirect
	github.com/sajari/fuzzy v1.0.0
	github.com/samalba/dockerclient v0.0.0-20160531175551-a30362618471 // indirect
	github.com/tent/http-link-go v0.0.0-20130702225549-ac974c61c2f9 // indirect
	github.com/tsuru/commandmocker v0.0.0-20160909010208-e1d28f4f616a
	github.com/tsuru/config v0.0.0-20201023175036-375aaee8b560
	github.com/tsuru/docker-cluster v0.0.0-20190325123005-f372d8d4e354
	github.com/tsuru/gandalf v0.0.0-20180117164358-86866cf0af24
	github.com/tsuru/gnuflag v0.0.0-20151217162021-86b8c1b864aa
	github.com/tsuru/go-gandalfclient v0.0.0-20200928142220-6d227717b7c3
	github.com/tsuru/monsterqueue v0.0.0-20160909010522-70e946ec66c3
	github.com/tsuru/tablecli v0.0.0-20190131152944-7ded8a3383c6
	github.com/uber/jaeger-client-go v2.25.0+incompatible
	github.com/uber/jaeger-lib v2.2.0+incompatible // indirect
	github.com/ugorji/go/codec v1.1.7
	github.com/vishvananda/netlink v1.0.0 // indirect
	github.com/vishvananda/netns v0.0.0-20190625233234-7109fa855b0f // indirect
	github.com/vmware/govcloudair v0.0.2 // indirect
	github.com/vmware/govmomi v0.0.0-20160923190800-b932baf416e9 // indirect
	github.com/vulcand/route v0.0.0-20191025171320-daa4df6c711a
	github.com/vulcand/vulcand v0.9.0
	golang.org/x/crypto v0.0.0-20201012173705-84dcc777aaee
	golang.org/x/net v0.0.0-20201021035429-f5854403a974
	golang.org/x/oauth2 v0.0.0-20191202225959-858c2ad4c8b6
	golang.org/x/sys v0.0.0-20210119212857-b64e53b001e4
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	gopkg.in/ahmetb/go-linq.v3 v3.0.0 // indirect
	gopkg.in/amz.v3 v3.0.0-20161215130849-8c3190dff075
	gopkg.in/bsm/ratelimit.v1 v1.0.0-20160220154919-db14e161995a // indirect
	gopkg.in/check.v1 v1.0.0-20200902074654-038fdea0a05b
	gopkg.in/mgo.v2 v2.0.0-20180705113604-9856a29383ce
	gopkg.in/redis.v3 v3.6.4
	gopkg.in/yaml.v2 v2.3.0
	k8s.io/api v0.18.9
	k8s.io/apiextensions-apiserver v0.18.9
	k8s.io/apimachinery v0.18.9
	k8s.io/autoscaler/vertical-pod-autoscaler v0.9.2
	k8s.io/client-go v0.18.9
	k8s.io/code-generator v0.18.9
	k8s.io/metrics v0.18.9
)

replace (
	github.com/docker/docker => github.com/docker/engine v0.0.0-20190219214528-cbe11bdc6da8
	github.com/samalba/dockerclient => github.com/cezarsa/dockerclient v0.0.0-20190924055524-af5052a88081
	gopkg.in/ahmetb/go-linq.v3 => github.com/ahmetb/go-linq v3.0.0+incompatible
	gopkg.in/check.v1 => gopkg.in/check.v1 v1.0.0-20161208181325-20d25e280405
)
