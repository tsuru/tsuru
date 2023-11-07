module github.com/tsuru/tsuru

go 1.20

require (
	github.com/ajg/form v0.0.0-20160822230020-523a5da1a92f
	github.com/bradfitz/go-smtpd v0.0.0-20130623174436-5b56f4f917c7
	github.com/codegangsta/negroni v0.0.0-20140611175843-a13766a8c257
	github.com/diego-araujo/go-saml v0.0.0-20151211102911-81203d242537
	github.com/docker/cli v23.0.0-rc.1+incompatible
	github.com/docker/docker v23.0.0-rc.1+incompatible
	github.com/elazarl/goproxy v0.0.0-20190711103511-473e67f1d7d2
	github.com/felixge/fgprof v0.9.1
	github.com/fsouza/go-dockerclient v1.7.4
	github.com/globalsign/mgo v0.0.0-20181015135952-eeefdecb41b8
	github.com/golang-jwt/jwt/v5 v5.0.0
	github.com/google/gops v0.0.0-20180311052415-160b358b10d6
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/websocket v1.4.2
	github.com/hashicorp/go-version v0.0.0-20180716215031-270f2f71b1ee
	github.com/imdario/mergo v0.3.12
	github.com/kr/pretty v0.3.0
	github.com/lestrrat-go/jwx/v2 v2.0.11
	github.com/mattn/go-shellwords v1.0.12
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d
	github.com/opentracing-contrib/go-stdlib v1.0.1-0.20201028152118-adbfc141dfc2
	github.com/opentracing/opentracing-go v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/pmorie/go-open-service-broker-client v0.0.0-20180330214919-dca737037ce6
	github.com/prometheus/client_golang v1.14.0
	github.com/prometheus/common v0.37.0
	github.com/robfig/cron/v3 v3.0.1
	github.com/rs/cors v1.9.0
	github.com/sajari/fuzzy v1.0.0
	github.com/tsuru/commandmocker v0.0.0-20160909010208-e1d28f4f616a
	github.com/tsuru/config v0.0.0-20201023175036-375aaee8b560
	github.com/tsuru/deploy-agent v0.0.0-20231017214502-c5b414b01059
	github.com/tsuru/gnuflag v0.0.0-20151217162021-86b8c1b864aa
	github.com/tsuru/tablecli v0.0.0-20190131152944-7ded8a3383c6
	github.com/uber/jaeger-client-go v2.25.0+incompatible
	github.com/ugorji/go/codec v1.1.7
	go.uber.org/automaxprocs v1.5.3
	golang.org/x/crypto v0.9.0
	golang.org/x/net v0.10.0
	golang.org/x/oauth2 v0.1.0
	golang.org/x/sys v0.8.0
	golang.org/x/term v0.8.0
	golang.org/x/text v0.9.0
	google.golang.org/grpc v1.50.1
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gopkg.in/mgo.v2 v2.0.0-20180705113604-9856a29383ce
	k8s.io/api v0.23.17
	k8s.io/apiextensions-apiserver v0.20.6
	k8s.io/apimachinery v0.23.17
	k8s.io/autoscaler/vertical-pod-autoscaler v0.9.2
	k8s.io/client-go v0.23.17
	k8s.io/code-generator v0.20.6
	k8s.io/ingress-gce v1.20.1
	k8s.io/metrics v0.20.6
	sigs.k8s.io/yaml v1.2.0
)

require (
	cloud.google.com/go v0.97.0 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/Microsoft/go-winio v0.6.0 // indirect
	github.com/RobotsAndPencils/go-saml v0.0.0-20150922030833-aa127de49a01 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/codahale/hdrhistogram v0.0.0-20161010025455-3a0bb77429bd // indirect
	github.com/containerd/containerd v1.6.16 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.2.0 // indirect
	github.com/docker/docker-credential-helpers v0.7.0 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/elazarl/goproxy/ext v0.0.0-20190711103511-473e67f1d7d2 // indirect
	github.com/emicklei/go-restful v2.9.5+incompatible // indirect
	github.com/evanphx/json-patch v4.12.0+incompatible // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.20.0 // indirect
	github.com/go-openapi/swag v0.19.5 // indirect
	github.com/goccy/go-json v0.10.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/pprof v0.0.0-20210720184732-4bb14d4b1be1 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/googleapis/gnostic v0.5.5 // indirect
	github.com/howeyc/fsnotify v0.9.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kardianos/osext v0.0.0-20151124170342-10da29423eb9 // indirect
	github.com/klauspost/compress v1.15.12 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/lestrrat-go/blackmagic v1.0.1 // indirect
	github.com/lestrrat-go/httpcc v1.0.1 // indirect
	github.com/lestrrat-go/httprc v1.0.4 // indirect
	github.com/lestrrat-go/iter v1.0.2 // indirect
	github.com/lestrrat-go/option v1.0.1 // indirect
	github.com/mailru/easyjson v0.7.0 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/moby/patternmatcher v0.5.0 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/moby/sys/sequential v0.5.0 // indirect
	github.com/moby/term v0.0.0-20221105221325-4eb28fa6025c // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0-rc2 // indirect
	github.com/opencontainers/runc v1.1.4 // indirect
	github.com/prometheus/client_model v0.3.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	github.com/rogpeppe/go-internal v1.6.1 // indirect
	github.com/segmentio/asm v1.2.0 // indirect
	github.com/sirupsen/logrus v1.9.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/uber/jaeger-lib v2.2.0+incompatible // indirect
	go.uber.org/atomic v1.4.0 // indirect
	golang.org/x/mod v0.8.0 // indirect
	golang.org/x/time v0.1.0 // indirect
	golang.org/x/tools v0.6.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20220706185917-7780775163c4 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/gengo v0.0.0-20210813121822-485abfe95c7c // indirect
	k8s.io/klog/v2 v2.60.1 // indirect
	k8s.io/kube-openapi v0.0.0-20211115234752-e816edb12b65 // indirect
	k8s.io/utils v0.0.0-20220210201930-3a6ce19ff2f9 // indirect
	sigs.k8s.io/json v0.0.0-20211020170558-c049b76a60c6 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
)

replace (
	github.com/ajg/form => github.com/cezarsa/form v0.0.0-20210510165411-863b166467b9
	github.com/samalba/dockerclient => github.com/cezarsa/dockerclient v0.0.0-20190924055524-af5052a88081
)
