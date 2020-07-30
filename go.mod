module github.com/tsuru/tsuru

go 1.12

require (
	github.com/RobotsAndPencils/go-saml v0.0.0-20150922030833-aa127de49a01 // indirect
	github.com/ajg/form v0.0.0-20160822230020-523a5da1a92f
	github.com/andrestc/docker-machine-driver-cloudstack v0.9.2
	github.com/armon/go-proxyproto v0.0.0-20190211145416-68259f75880e // indirect
	github.com/aws/aws-sdk-go v1.25.48
	github.com/bradfitz/go-smtpd v0.0.0-20130623174436-5b56f4f917c7
	github.com/codegangsta/cli v1.19.1 // indirect
	github.com/codegangsta/negroni v0.0.0-20140611175843-a13766a8c257
	github.com/containerd/continuity v0.0.0-20190426062206-aaeac12a7ffc // indirect
	github.com/diego-araujo/go-saml v0.0.0-20151211102911-81203d242537
	github.com/digitalocean/godo v1.6.0
	github.com/dnaeon/go-vcr v1.0.1 // indirect
	github.com/docker/docker v1.5.0
	github.com/docker/libnetwork v0.8.0-dev.2.0.20180706232811-d00ceed44cc4 // indirect
	github.com/docker/machine v0.7.0
	github.com/elazarl/goproxy v0.0.0-20190711103511-473e67f1d7d2 // indirect
	github.com/elazarl/goproxy/ext v0.0.0-20190711103511-473e67f1d7d2 // indirect
	github.com/exoscale/egoscale v0.9.31 // indirect
	github.com/fsouza/go-dockerclient v0.0.0-20180427001620-3a206030a28a
	github.com/garyburd/redigo v1.6.0 // indirect
	github.com/globalsign/mgo v0.0.0-20181015135952-eeefdecb41b8
	github.com/go-ini/ini v1.41.0 // indirect
	github.com/google/gops v0.0.0-20180311052415-160b358b10d6
	github.com/gorilla/mux v1.7.3
	github.com/gorilla/pat v0.0.0-20131205071617-ae2e162c4b2a // indirect
	github.com/gorilla/websocket v1.4.0
	github.com/hashicorp/go-version v1.1.0
	github.com/heptio/authenticator v0.3.0 // indirect
	github.com/intel-go/cpuid v0.0.0-20181003105527-1a4a6f06a1c6 // indirect
	github.com/jinzhu/copier v0.0.0-20180308034124-7e38e58719c3 // indirect
	github.com/kardianos/osext v0.0.0-20151124170342-10da29423eb9 // indirect
	github.com/kr/pretty v0.1.0
	github.com/mailgun/holster v3.0.0+incompatible // indirect
	github.com/mailgun/metrics v0.0.0-20170714162148-fd99b46995bd // indirect
	github.com/mailgun/minheap v0.0.0-20170619185613-3dbe6c6bf55f // indirect
	github.com/mailgun/multibuf v0.0.0-20150714184110-565402cd71fb // indirect
	github.com/mailgun/timetools v0.0.0-20170619190023-f3a7b8ffff47 // indirect
	github.com/mailgun/ttlmap v0.0.0-20170619185759-c1c17f74874f // indirect
	github.com/mattn/go-shellwords v1.0.2
	github.com/mcuadros/go-version v0.0.0-20190308113854-92cdf37c5b75 // indirect
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d
	github.com/pkg/errors v0.8.1
	github.com/pmorie/go-open-service-broker-client v0.0.0-20180330214919-dca737037ce6
	github.com/prometheus/client_golang v1.4.0
	github.com/prometheus/client_model v0.2.0
	github.com/rackspace/gophercloud v0.0.0-20160825135439-c90cb954266e // indirect
	github.com/rancher/kontainer-engine v0.0.0-00010101000000-000000000000
	github.com/rancher/rke v1.1.0-rc9.0.20200327175519-ecc629f2c3d5
	github.com/rancher/types v0.0.0-20200326224235-0d1e1dcc8d55
	github.com/sajari/fuzzy v0.0.0-20141008071338-bbbcac964e38
	github.com/samalba/dockerclient v0.0.0-20160531175551-a30362618471 // indirect
	github.com/tsuru/commandmocker v0.0.0-20160909010208-e1d28f4f616a
	github.com/tsuru/config v0.0.0-20200717192526-2a9a0efe5f28
	github.com/tsuru/docker-cluster v0.0.0-20190325123005-f372d8d4e354
	github.com/tsuru/gandalf v0.0.0-20180117164358-86866cf0af24
	github.com/tsuru/gnuflag v0.0.0-20151217162021-86b8c1b864aa
	github.com/tsuru/go-gandalfclient v0.0.0-20160909010455-9d375d012f75
	github.com/tsuru/monsterqueue v0.0.0-20160909010522-70e946ec66c3
	github.com/tsuru/tablecli v0.0.0-20190131152944-7ded8a3383c6
	github.com/ugorji/go/codec v1.1.7
	github.com/vishvananda/netlink v1.0.0 // indirect
	github.com/vishvananda/netns v0.0.0-20190625233234-7109fa855b0f // indirect
	github.com/vmware/govcloudair v0.0.2 // indirect
	github.com/vmware/govmomi v0.0.0-20160923190800-b932baf416e9 // indirect
	github.com/vulcand/oxy v0.0.0-20180707144047-21cae4f7b50b // indirect
	github.com/vulcand/predicate v0.0.0-20141020235656-cb0bff91a7ab // indirect
	github.com/vulcand/route v0.0.0-20181101151700-58b44163b968
	github.com/vulcand/vulcand v0.0.0-20181107172627-5fb2302c78e2
	golang.org/x/crypto v0.0.0-20200220183623-bac4c82f6975
	golang.org/x/net v0.0.0-20191112182307-2180aed22343
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	golang.org/x/sys v0.0.0-20200122134326-e047566fdf82
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0
	gopkg.in/ahmetb/go-linq.v3 v3.0.0-00010101000000-000000000000 // indirect
	gopkg.in/amz.v3 v3.0.0-20161215130849-8c3190dff075
	gopkg.in/bsm/ratelimit.v1 v1.0.0-20160220154919-db14e161995a // indirect
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f
	gopkg.in/redis.v3 v3.6.4
	gopkg.in/yaml.v2 v2.3.0
	k8s.io/api v0.18.6
	k8s.io/apiextensions-apiserver v0.18.6
	k8s.io/apimachinery v0.18.6
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/code-generator v0.18.6
	k8s.io/kubectl v0.18.0
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v12.2.0+incompatible
	github.com/docker/docker => github.com/docker/engine v0.0.0-20190219214528-cbe11bdc6da8
	github.com/docker/machine => github.com/tsuru/machine v0.7.1-0.20190219165632-cdcfd549f935
	github.com/rancher/kontainer-engine => github.com/cezarsa/kontainer-engine v0.0.4-dev.0.20200730142449-fd5608eed285
	github.com/samalba/dockerclient => github.com/cezarsa/dockerclient v0.0.0-20190924055524-af5052a88081
	gopkg.in/ahmetb/go-linq.v3 => github.com/ahmetb/go-linq v3.0.0+incompatible
	gopkg.in/check.v1 => gopkg.in/check.v1 v1.0.0-20161208181325-20d25e280405
	k8s.io/client-go => k8s.io/client-go v0.18.6
)
