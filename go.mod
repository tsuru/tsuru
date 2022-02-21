module github.com/tsuru/tsuru

go 1.12

require (
	github.com/RobotsAndPencils/go-saml v0.0.0-20150922030833-aa127de49a01 // indirect
	github.com/ajg/form v0.0.0-20160822230020-523a5da1a92f
	github.com/andrestc/docker-machine-driver-cloudstack v0.9.2
	github.com/aws/aws-sdk-go v1.28.2
	github.com/bradfitz/go-smtpd v0.0.0-20130623174436-5b56f4f917c7
	github.com/cenkalti/backoff v0.0.0-20160904140958-8edc80b07f38 // indirect
	github.com/codahale/hdrhistogram v0.0.0-20161010025455-3a0bb77429bd // indirect
	github.com/codegangsta/cli v1.19.1 // indirect
	github.com/codegangsta/negroni v0.0.0-20140611175843-a13766a8c257
	github.com/diego-araujo/go-saml v0.0.0-20151211102911-81203d242537
	github.com/digitalocean/godo v0.0.0-20170404195252-dfa802149cae
	github.com/docker/cli v20.10.12+incompatible
	github.com/docker/docker v20.10.8+incompatible
	github.com/docker/docker-credential-helpers v0.6.4 // indirect
	github.com/docker/machine v0.7.1-0.20190902101342-b170508bf44c
	github.com/elazarl/goproxy v0.0.0-20190711103511-473e67f1d7d2
	github.com/elazarl/goproxy/ext v0.0.0-20190711103511-473e67f1d7d2 // indirect
	github.com/exoscale/egoscale v0.9.23 // indirect
	github.com/felixge/fgprof v0.9.1
	github.com/fsouza/go-dockerclient v1.7.4
	github.com/garyburd/redigo v1.6.0 // indirect
	github.com/ghodss/yaml v1.0.0
	github.com/globalsign/mgo v0.0.0-20181015135952-eeefdecb41b8
	github.com/google/go-querystring v0.0.0-20150414214848-547ef5ac9797 // indirect
	github.com/google/gops v0.0.0-20180311052415-160b358b10d6
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/websocket v1.4.2
	github.com/hashicorp/go-version v0.0.0-20180716215031-270f2f71b1ee
	github.com/intel-go/cpuid v0.0.0-20181003105527-1a4a6f06a1c6 // indirect
	github.com/jinzhu/copier v0.0.0-20180308034124-7e38e58719c3 // indirect
	github.com/kardianos/osext v0.0.0-20151124170342-10da29423eb9 // indirect
	github.com/kr/pretty v0.3.0
	github.com/mattn/go-shellwords v1.0.3
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/opencontainers/runc v1.0.2 // indirect
	github.com/opentracing-contrib/go-stdlib v1.0.1-0.20201028152118-adbfc141dfc2
	github.com/opentracing/opentracing-go v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/pmorie/go-open-service-broker-client v0.0.0-20180330214919-dca737037ce6
	github.com/prometheus/client_golang v1.7.1
	github.com/prometheus/common v0.10.0
	github.com/rackspace/gophercloud v0.0.0-20160825135439-c90cb954266e // indirect
	github.com/sajari/fuzzy v1.0.0
	github.com/samalba/dockerclient v0.0.0-20160531175551-a30362618471 // indirect
	github.com/tent/http-link-go v0.0.0-20130702225549-ac974c61c2f9 // indirect
	github.com/tsuru/commandmocker v0.0.0-20160909010208-e1d28f4f616a
	github.com/tsuru/config v0.0.0-20201023175036-375aaee8b560
	github.com/tsuru/docker-cluster v0.0.0-20190325123005-f372d8d4e354
	github.com/tsuru/gnuflag v0.0.0-20151217162021-86b8c1b864aa
	github.com/tsuru/monsterqueue v0.0.0-20160909010522-70e946ec66c3
	github.com/tsuru/tablecli v0.0.0-20190131152944-7ded8a3383c6
	github.com/uber/jaeger-client-go v2.25.0+incompatible
	github.com/uber/jaeger-lib v2.2.0+incompatible // indirect
	github.com/ugorji/go/codec v1.1.7
	github.com/vmware/govcloudair v0.0.2 // indirect
	golang.org/x/crypto v0.0.0-20210322153248-0c34fe9e7dc2
	golang.org/x/net v0.0.0-20210503060351-7fd8e65b6420
	golang.org/x/oauth2 v0.0.0-20210427180440-81ed05c6b58c
	golang.org/x/sys v0.0.0-20210616094352-59db8d763f22
	golang.org/x/term v0.0.0-20210615171337-6886f2dfbf5b
	golang.org/x/tools v0.1.1 // indirect
	gopkg.in/amz.v3 v3.0.0-20161215130849-8c3190dff075
	gopkg.in/bsm/ratelimit.v1 v1.0.0-20160220154919-db14e161995a // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gopkg.in/mgo.v2 v2.0.0-20180705113604-9856a29383ce
	gopkg.in/redis.v3 v3.6.4
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.20.6
	k8s.io/apiextensions-apiserver v0.20.6
	k8s.io/apimachinery v0.20.6
	k8s.io/autoscaler/vertical-pod-autoscaler v0.9.2
	k8s.io/client-go v0.20.6
	k8s.io/code-generator v0.20.6
	k8s.io/ingress-gce v1.13.1-0.20210721173208-ffd5b5d20da1
	k8s.io/metrics v0.20.6
	launchpad.net/gocheck v0.0.0-20140225173054-000000000087 // indirect
)

replace (
	github.com/ajg/form => github.com/cezarsa/form v0.0.0-20210510165411-863b166467b9
	github.com/samalba/dockerclient => github.com/cezarsa/dockerclient v0.0.0-20190924055524-af5052a88081
)
