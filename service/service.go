// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db/storagev2"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	jobTypes "github.com/tsuru/tsuru/types/job"
	provTypes "github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/validation"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type ServiceEncoding string

var (
	ServiceEncodingDefault = ServiceEncoding("") // default is form
	ServiceEncodingForm    = ServiceEncoding("form")
	ServiceEncodingJSON    = ServiceEncoding("json")
)

type Service struct {
	Name         string `bson:"_id"`
	Username     string
	Password     string
	Endpoint     map[string]string
	OwnerTeams   []string `bson:"owner_teams"`
	Teams        []string
	Doc          string
	IsRestricted bool `bson:"is_restricted"`
	// IsMultiCluster indicates whether Service Instances (children of this Service)
	// run within the user's Cluster (same pool of Tsuru Apps). When enabled, creating
	// a Service Instance must require a valid Pool.
	//
	// This field is immutable (after creating Service).
	IsMultiCluster bool `bson:"is_multi_cluster"`

	// Encoding defines how modern is the backend
	// Tsuru started with a form encoding, but some services may support a more modern JSON encoding.
	Encoding ServiceEncoding  `bson:"encoding"`
	// Manifest defines the provider operations that can be matched for fine-grained authorization.
	Manifest *ServiceManifest `bson:"manifest,omitempty" json:"manifest,omitempty"`
}

// ServiceManifest describes the provider operations exposed by a service for
// fine-grained authorization checks.
type ServiceManifest struct {
	Enabled         bool                `bson:"enabled" json:"enabled"`
	StrictActions   bool                `bson:"strict_actions" json:"strict_actions"`
	LegacyCompat    bool                `bson:"legacy_compat" json:"legacy_compat"`
	LegacyEnabledAt *time.Time          `bson:"legacy_enabled_at,omitempty" json:"legacy_enabled_at,omitempty"`
	Operations      []ManifestOperation `bson:"operations" json:"operations"`

	matchOnce sync.Once                   `bson:"-" json:"-"`
	matchErr  error                       `bson:"-" json:"-"`
	matchOps  []compiledManifestOperation `bson:"-" json:"-"`
}

// ManifestOperation maps an HTTP method and path template to an authorization action.
type ManifestOperation struct {
	Name       string `bson:"name" json:"name"`
	Method     string `bson:"method" json:"method"`
	Path       string `bson:"path" json:"path"`
	Action     string `bson:"action" json:"action"`
	Scope      string `bson:"scope" json:"scope"`
	EntityType string `bson:"entity_type,omitempty" json:"entity_type,omitempty"`
}

type compiledManifestOperation struct {
	action string
	method string
	path   *regexp.Regexp
	score  manifestOperationScore
	index  int
}

type manifestOperationScore struct {
	segments        int
	literalSegments int
	literalLength   int
	variableCount   int
}

type BindAppParameters map[string]interface{}

type ProxyOpts struct {
	Instance  *ServiceInstance
	Path      string
	Event     *event.Event
	RequestID string
	Writer    http.ResponseWriter
	Request   *http.Request
}

// Match resolves the manifest action for the given method and path.
func (m *ServiceManifest) Match(method, rawPath string) (string, bool) {
	if m == nil {
		return "", false
	}
	ops, err := m.compiledOperations()
	if err != nil {
		return "", false
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	matchPath := normalizeManifestPath(rawPath)
	for _, op := range ops {
		if op.method != method {
			continue
		}
		if op.path.MatchString(matchPath) {
			return op.action, true
		}
	}
	return "", false
}

func (m *ServiceManifest) compiledOperations() ([]compiledManifestOperation, error) {
	m.matchOnce.Do(func() {
		ops := make([]compiledManifestOperation, 0, len(m.Operations))
		for i, op := range m.Operations {
			compiled, err := compileManifestOperation(op, i)
			if err != nil {
				m.matchErr = err
				return
			}
			ops = append(ops, compiled)
		}
		// More specific paths are matched first so overlapping templates resolve deterministically.
		sort.SliceStable(ops, func(i, j int) bool {
			return compareManifestOperationScore(ops[i], ops[j])
		})
		m.matchOps = ops
	})
	return m.matchOps, m.matchErr
}

func compileManifestOperation(op ManifestOperation, index int) (compiledManifestOperation, error) {
	normalizedPath := normalizeManifestPath(op.Path)
	route := mux.NewRouter().NewRoute().Path(normalizedPath)
	pathExpr, err := route.GetPathRegexp()
	if err != nil {
		return compiledManifestOperation{}, err
	}
	pathRegexp, err := regexp.Compile(pathExpr)
	if err != nil {
		return compiledManifestOperation{}, err
	}
	return compiledManifestOperation{
		action: op.Action,
		method: strings.ToUpper(strings.TrimSpace(op.Method)),
		path:   pathRegexp,
		score:  manifestPathScore(normalizedPath),
		index:  index,
	}, nil
}

func compareManifestOperationScore(left, right compiledManifestOperation) bool {
	// Precedence favors longer paths, then more literal structure, then earlier declaration order.
	if left.score.segments != right.score.segments {
		return left.score.segments > right.score.segments
	}
	if left.score.literalSegments != right.score.literalSegments {
		return left.score.literalSegments > right.score.literalSegments
	}
	if left.score.literalLength != right.score.literalLength {
		return left.score.literalLength > right.score.literalLength
	}
	if left.score.variableCount != right.score.variableCount {
		return left.score.variableCount < right.score.variableCount
	}
	return left.index < right.index
}

func manifestPathScore(rawPath string) manifestOperationScore {
	normalizedPath := normalizeManifestPath(rawPath)
	trimmed := strings.Trim(normalizedPath, "/")
	if trimmed == "" {
		return manifestOperationScore{}
	}
	segments := strings.Split(trimmed, "/")
	score := manifestOperationScore{segments: len(segments)}
	for _, segment := range segments {
		if isManifestVariableSegment(segment) {
			score.variableCount++
			continue
		}
		score.literalSegments++
		score.literalLength += len(segment)
	}
	return score
}

func normalizeManifestPath(rawPath string) string {
	cleaned := strings.TrimSpace(rawPath)
	if cleaned == "" {
		return "/"
	}
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	cleaned = path.Clean(cleaned)
	if cleaned == "." {
		return "/"
	}
	return cleaned
}

func isManifestVariableSegment(segment string) bool {
	return strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}")
}

// TODO: use requestID inside the context
type ServiceClient interface {
	Create(ctx context.Context, instance *ServiceInstance, evt *event.Event, requestID string) error
	Update(ctx context.Context, instance *ServiceInstance, evt *event.Event, requestID string) error
	Destroy(ctx context.Context, instance *ServiceInstance, evt *event.Event, requestID string) error
	BindApp(ctx context.Context, instance *ServiceInstance, app *appTypes.App, params BindAppParameters, evt *event.Event, requestID string) (map[string]string, error)
	BindJob(ctx context.Context, instance *ServiceInstance, job *jobTypes.Job, evt *event.Event, requestID string) (map[string]string, error)
	UnbindApp(ctx context.Context, instance *ServiceInstance, app *appTypes.App, evt *event.Event, requestID string) error
	UnbindJob(ctx context.Context, instance *ServiceInstance, job *jobTypes.Job, evt *event.Event, requestID string) error
	Status(ctx context.Context, instance *ServiceInstance, requestID string) (string, error)
	Info(ctx context.Context, instance *ServiceInstance, requestID string) ([]map[string]string, error)
	Plans(ctx context.Context, pool, requestID string) ([]Plan, error)
	Proxy(ctx context.Context, opts *ProxyOpts) error
}

var (
	ErrServiceAlreadyExists = errors.New("Service already exists.")
	ErrServiceNotFound      = errors.New("Service not found.")
	ErrMissingPool          = errors.New("Missing pool")

	schemeRegexp = regexp.MustCompile("^https?://")
)

func Get(ctx context.Context, service string) (Service, error) {
	collection, err := storagev2.ServicesCollection()
	if err != nil {
		return Service{}, err
	}
	var s Service
	if err := collection.FindOne(ctx, mongoBSON.M{"_id": service}).Decode(&s); err != nil {
		if err == mongo.ErrNoDocuments {
			return Service{}, ErrServiceNotFound
		}
		return Service{}, err
	}
	return s, nil
}

func Create(ctx context.Context, s Service) error {
	if err := s.validate(ctx, false); err != nil {
		return err
	}
	collection, err := storagev2.ServicesCollection()
	if err != nil {
		return err
	}
	n, err := collection.CountDocuments(ctx, mongoBSON.M{"_id": s.Name})
	if err != nil {
		return err
	}
	if n != 0 {
		return ErrServiceAlreadyExists
	}
	_, err = collection.InsertOne(ctx, s)

	return err
}

func Update(ctx context.Context, s Service) error {
	if err := s.validate(ctx, true); err != nil {
		return err
	}
	collection, err := storagev2.ServicesCollection()
	if err != nil {
		return err
	}
	_, err = collection.ReplaceOne(ctx, mongoBSON.M{"_id": s.Name}, s)
	if err == mongo.ErrNoDocuments {
		return ErrServiceNotFound
	}
	return err
}

func Delete(ctx context.Context, s Service) error {
	collection, err := storagev2.ServicesCollection()
	if err != nil {
		return err
	}
	result, err := collection.DeleteOne(ctx, mongoBSON.M{"_id": s.Name})
	if err == mongo.ErrNoDocuments {
		return ErrServiceNotFound
	}
	if err != nil {
		return err
	}

	if result.DeletedCount == 0 {
		return ErrServiceNotFound
	}

	return nil
}

func GetServices(ctx context.Context) ([]Service, error) {
	return getServicesByFilter(ctx, nil)
}

func GetServicesByTeamsAndServices(ctx context.Context, teams []string, services []string) ([]Service, error) {
	var filter mongoBSON.M
	if teams != nil || services != nil {
		orFilter := []mongoBSON.M{
			{"is_restricted": false},
		}

		if teams != nil {
			orFilter = append(orFilter, mongoBSON.M{"teams": mongoBSON.M{"$in": teams}})
		}
		if services != nil {
			orFilter = append(orFilter, mongoBSON.M{"_id": mongoBSON.M{"$in": services}})
		}

		if len(orFilter) > 1 {
			filter = mongoBSON.M{
				"$or": orFilter,
			}
		} else if len(orFilter) == 1 {
			filter = orFilter[0]
		}
	}
	return getServicesByFilter(ctx, filter)
}

func GetServicesByOwnerTeamsAndServices(ctx context.Context, teams []string, services []string) ([]Service, error) {
	var filter mongoBSON.M
	if teams != nil || services != nil {
		orFilter := []mongoBSON.M{}

		if teams != nil {
			orFilter = append(orFilter, mongoBSON.M{"owner_teams": mongoBSON.M{"$in": teams}})
		}
		if services != nil {
			orFilter = append(orFilter, mongoBSON.M{"_id": mongoBSON.M{"$in": services}})
		}

		if len(orFilter) > 1 {
			filter = mongoBSON.M{
				"$or": orFilter,
			}
		} else if len(orFilter) == 1 {
			filter = orFilter[0]
		}
	}
	return getServicesByFilter(ctx, filter)
}

func RenameServiceTeam(ctx context.Context, oldName, newName string) error {
	collection, err := storagev2.ServicesCollection()
	if err != nil {
		return err
	}

	models := []mongo.WriteModel{}

	for _, field := range []string{"owner_teams", "teams"} {
		models = append(models,
			mongo.NewUpdateManyModel().
				SetFilter(mongoBSON.M{field: oldName}).
				SetUpdate(mongoBSON.M{"$addToSet": mongoBSON.M{field: newName}}),

			mongo.NewUpdateManyModel().
				SetFilter(mongoBSON.M{field: oldName}).
				SetUpdate(mongoBSON.M{"$pull": mongoBSON.M{field: oldName}}),
		)
	}

	_, err = collection.BulkWrite(ctx, models)
	if err != nil {
		return err
	}

	return nil
}

func getServicesByFilter(ctx context.Context, filter mongoBSON.M) ([]Service, error) {
	collection, err := storagev2.ServicesCollection()
	if err != nil {
		return nil, err
	}
	if filter == nil {
		filter = mongoBSON.M{}
	}
	var services []Service
	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}

	err = cursor.All(ctx, &services)
	if err != nil {
		return nil, err
	}

	return services, err
}

func (s *Service) HasTeam(team *authTypes.Team) bool {
	return s.findTeam(team) > -1
}

func (s *Service) GrantAccess(team *authTypes.Team) error {
	if s.HasTeam(team) {
		return errors.New("This team already has access to this service")
	}
	s.Teams = append(s.Teams, team.Name)
	return nil
}

func (s *Service) RevokeAccess(team *authTypes.Team) error {
	index := s.findTeam(team)
	if index < 0 {
		return errors.New("This team does not have access to this service")
	}
	copy(s.Teams[index:], s.Teams[index+1:])
	s.Teams = s.Teams[:len(s.Teams)-1]
	return nil
}

func (s *Service) getUsername() string {
	if s.Username != "" {
		return s.Username
	}
	return s.Name
}

func (s *Service) findTeam(team *authTypes.Team) int {
	for i, t := range s.Teams {
		if team.Name == t {
			return i
		}
	}
	return -1
}

func endpointNameForPool(ctx context.Context, pool string) (string, error) {
	if pool == "" {
		return "", nil
	}
	p, err := servicemanager.Pool.FindByName(ctx, pool)
	if err != nil {
		return "", err
	}
	c, err := servicemanager.Cluster.FindByPool(ctx, p.Provisioner, p.Name)
	if err != nil {
		if err == provTypes.ErrNoCluster {
			return "", nil
		}
		return "", err
	}
	return c.Name, nil
}

func (s *Service) getClientForPool(ctx context.Context, pool string) (ServiceClient, error) {
	var cli ServiceClient
	poolEndpoint, err := endpointNameForPool(ctx, pool)
	if err != nil {
		return cli, err
	}
	var endpoints []string
	if poolEndpoint != "" {
		endpoints = []string{poolEndpoint, "production"}
	} else {
		endpoints = []string{"production"}
	}
	return s.getClient(endpoints...)
}

func (s *Service) getClient(endpoints ...string) (ServiceClient, error) {
	var err error
	for _, endpoint := range endpoints {
		if e, ok := s.Endpoint[endpoint]; ok {
			if p := schemeRegexp.MatchString(e); !p {
				e = "http://" + e
			}
			cli := &endpointClient{
				serviceName: s.Name,
				endpoint:    e,
				username:    s.getUsername(),
				password:    s.Password,
				encoding:    s.Encoding,
			}
			return cli, nil
		} else {
			err = errors.New("Unknown endpoint: " + endpoint)
		}
	}
	return nil, err
}

func (s *Service) validate(ctx context.Context, skipName bool) (err error) {
	defer func() {
		if err != nil {
			err = &tsuruErrors.ValidationError{Message: err.Error()}
		}
	}()
	if s.Name == "" {
		return fmt.Errorf("Service id is required")
	}
	if !skipName && !validation.ValidateName(s.Name) {
		return fmt.Errorf("Invalid service id, should have at most 40 " +
			"characters, containing only lower case letters, numbers or dashes, " +
			"starting with a letter.")
	}
	if s.Password == "" {
		return fmt.Errorf("Service password is required")
	}
	if len(s.Endpoint) == 0 {
		return fmt.Errorf("At least one endpoint is required")
	}
	return s.validateOwnerTeams(ctx)
}

func (s *Service) validateOwnerTeams(ctx context.Context) error {
	if len(s.OwnerTeams) == 0 {
		return fmt.Errorf("At least one service team owner is required")
	}
	teams, err := servicemanager.Team.FindByNames(ctx, s.OwnerTeams)
	if err != nil {
		return nil
	}
	if len(teams) != len(s.OwnerTeams) {
		return fmt.Errorf("Team owner doesn't exist")
	}
	return nil
}

func getServicesNames(services []Service) []string {
	sNames := make([]string, len(services))
	for i, s := range services {
		sNames[i] = s.Name
	}
	return sNames
}

type ServiceModel struct {
	Service          string            `json:"service"`
	Instances        []string          `json:"instances"`
	Plans            []string          `json:"plans"`
	ServiceInstances []ServiceInstance `json:"service_instances"`
}

// Proxy is a proxy between tsuru and the service.
// This method allow customized service methods.
func Proxy(ctx context.Context, service *Service, path string, evt *event.Event, requestID string, w http.ResponseWriter, r *http.Request) error {
	endpoint, err := service.getClient("production")
	if err != nil {
		return err
	}
	return endpoint.Proxy(ctx, &ProxyOpts{
		Path:      path,
		Event:     evt,
		RequestID: requestID,
		Writer:    w,
		Request:   r,
	})
}
