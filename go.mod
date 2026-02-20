module github.com/tsuru/tsuru

go 1.24.2

require (
	github.com/adhocore/gronx v1.6.6
	github.com/bradfitz/go-smtpd v0.0.0-20130623174436-5b56f4f917c7
	github.com/cezarsa/form v0.0.0-20210510165411-863b166467b9
	github.com/codegangsta/negroni v0.0.0-20140611175843-a13766a8c257
	github.com/docker/cli v23.0.3+incompatible
	github.com/docker/docker v28.0.0+incompatible
	github.com/elazarl/goproxy v1.2.1
	github.com/felixge/fgprof v0.9.1
	github.com/fsnotify/fsnotify v1.9.0
	github.com/golang-jwt/jwt/v5 v5.2.2
	github.com/google/gops v0.0.0-20180311052415-160b358b10d6
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/websocket v1.5.3
	github.com/imdario/mergo v0.3.16
	github.com/kedacore/keda/v2 v2.17.3
	github.com/kr/pretty v0.3.1
	github.com/lestrrat-go/jwx/v2 v2.0.21
	github.com/mattn/go-shellwords v1.0.12
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.21.1
	github.com/prometheus/common v0.63.0
	github.com/robfig/cron/v3 v3.0.1
	github.com/rs/cors v1.11.0
	github.com/sajari/fuzzy v1.0.0
	github.com/stretchr/testify v1.11.1
	github.com/tsuru/commandmocker v0.0.0-20160909010208-e1d28f4f616a
	github.com/tsuru/config v0.0.0-20201023175036-375aaee8b560
	github.com/tsuru/deploy-agent v0.0.0-20241004132402-8b6d39f21671
	github.com/tsuru/gnuflag v0.0.0-20151217162021-86b8c1b864aa
	github.com/tsuru/tablecli v0.0.0-20190131152944-7ded8a3383c6
	github.com/ugorji/go/codec v1.1.7
	go.mongodb.org/mongo-driver v1.17.3
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.65.0
	go.opentelemetry.io/otel v1.40.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.40.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.40.0
	go.opentelemetry.io/otel/sdk v1.40.0
	go.opentelemetry.io/otel/trace v1.40.0
	go.uber.org/automaxprocs v1.6.0
	golang.org/x/crypto v0.47.0
	golang.org/x/net v0.49.0
	golang.org/x/oauth2 v0.34.0
	golang.org/x/term v0.39.0
	golang.org/x/text v0.33.0
	google.golang.org/grpc v1.78.0
	google.golang.org/protobuf v1.36.11
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	k8s.io/api v0.32.2
	k8s.io/apiextensions-apiserver v0.32.1
	k8s.io/apimachinery v0.32.2
	k8s.io/autoscaler/vertical-pod-autoscaler v0.9.2
	k8s.io/client-go v0.32.2
	k8s.io/code-generator v0.32.2
	k8s.io/ingress-gce v1.20.1
	k8s.io/metrics v0.31.2
	k8s.io/utils v0.0.0-20241210054802-24370beab758
	sigs.k8s.io/yaml v1.4.0
)

require (
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/expr-lang/expr v1.17.7 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fxamacker/cbor/v2 v2.7.0 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/gnostic-models v0.6.9 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.7 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/metric v1.40.0 // indirect
	go.opentelemetry.io/proto/otlp v1.9.0 // indirect
	golang.org/x/exp v0.0.0-20250210185358-939b2ce775ac // indirect
	golang.org/x/tools/go/packages/packagestest v0.1.1-deprecated // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260128011058-8636f8732409 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260128011058-8636f8732409 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.12.0 // indirect
	gotest.tools/v3 v3.5.2 // indirect
	k8s.io/gengo/v2 v2.0.0-20240911193312-2b36238f13e9 // indirect
)

require (
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20230124172434-306776ec8161 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/containerd/containerd v1.7.29
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.2.0 // indirect
	github.com/docker/docker-credential-helpers v0.7.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/emicklei/go-restful/v3 v3.12.1 // indirect
	github.com/evanphx/json-patch/v5 v5.9.11 // indirect
	github.com/globocom/mongo-go-prometheus v0.1.1
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/jsonreference v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/goccy/go-json v0.10.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/pprof v0.0.0-20250403155104-27863c87afa6 // indirect
	github.com/howeyc/fsnotify v0.9.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kardianos/osext v0.0.0-20151124170342-10da29423eb9 // indirect
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/lestrrat-go/blackmagic v1.0.2 // indirect
	github.com/lestrrat-go/httpcc v1.0.1 // indirect
	github.com/lestrrat-go/httprc v1.0.5 // indirect
	github.com/lestrrat-go/iter v1.0.2 // indirect
	github.com/lestrrat-go/option v1.0.1 // indirect
	github.com/mailru/easyjson v0.9.0 // indirect
	github.com/moby/spdystream v0.5.0 // indirect
	github.com/moby/term v0.5.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/montanaflynn/stats v0.7.1 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/segmentio/asm v1.2.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.1.2 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	golang.org/x/mod v0.31.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/time v0.12.0 // indirect
	golang.org/x/tools v0.40.0 // indirect
	gomodules.xyz/jsonpatch/v2 v2.4.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/kube-openapi v0.0.0-20241212222426-2c72e554b1e7 // indirect
	knative.dev/pkg v0.0.0-20250326102644-9f3e60a9244c // indirect
	sigs.k8s.io/controller-runtime v0.19.7 // indirect
	sigs.k8s.io/json v0.0.0-20241014173422-cfa47c3a1cc8 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.5.0 // indirect
)
