package alertmanager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strconv"
	"sync/atomic"

	"github.com/influxdata/kapacitor/alert"
	"github.com/influxdata/kapacitor/keyvalue"
	"github.com/pkg/errors"
	"github.com/satori/go.uuid"
)

type Service struct {
	configValue atomic.Value
	diag        Diagnostic
}

type Diagnostic interface {
	WithContext(ctx ...keyvalue.T) Diagnostic
	TemplateError(err error, kv keyvalue.T)
	Error(msg string, err error)
	Debug(msg string)
}

func NewService(c Config, d Diagnostic) *Service {
	s := &Service{
		diag: d,
	}
	s.configValue.Store(c)
	return s
}

func (s *Service) Open() error {
	// Perform any initialization needed here
	return nil
}

func (s *Service) Close() error {
	// Perform any actions needed to properly close the service here.
	// For example signal and wait for all go routines to finish.
	return nil
}

func (s *Service) Update(newConfig []interface{}) error {
	if l := len(newConfig); l != 1 {
		return fmt.Errorf("expected only one new config object, got %d", l)
	}
	if c, ok := newConfig[0].(Config); !ok {
		return fmt.Errorf("expected config object to be of type %T, got %T", c, newConfig[0])
	} else {
		s.configValue.Store(c)
	}
	return nil
}

// config loads the config struct stored in the configValue field.
func (s *Service) config() Config {
	return s.configValue.Load().(Config)
}

// Alert sends a message to the specified room.
func (s *Service) Alert(url, retryFolder string, event alert.Event) error {
	c := s.config()
	if !c.Enabled {
		return errors.New("service is not enabled")
	}
	type AlertManagerEvent struct {
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
	}
	amEvent := AlertManagerEvent{
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
	}

	// add global tags
	amEvent.Labels["_topic"] = event.Topic
	amEvent.Labels["_ID"] = event.State.ID
	amEvent.Labels["_message"] = event.State.Message
	amEvent.Labels["_level"] = event.State.Level.String()
	amEvent.Labels["_name"] = event.Data.Name
	amEvent.Labels["_taskName"] = event.Data.TaskName
	amEvent.Labels["_category"] = event.Data.Category
	amEvent.Labels["_recoverable"] = strconv.FormatBool(event.Data.Recoverable)

	// add tags
	for k, v := range event.Data.Tags {
		amEvent.Labels[k] = v
	}

	// add fields as annotations
	for k, v := range event.Data.Fields {
		amEvent.Annotations[k] = v.(string)
	}

	data, err := json.Marshal([]AlertManagerEvent{amEvent})
	if err != nil {
		return err
	}
	r, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		// write json for retry only it couldn't be posted
		save_err := s.saveJSON(retryFolder, data)
		if save_err != nil {
			s.diag.Error("Couldn't save alert for retry", save_err)
		}
		return err
	}
	r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response code %d from AlertManager service", r.StatusCode)
	}
	return nil
}

func (s *Service) saveJSON(retryFolder string, json []byte) error {
	file_id, uuid_err := uuid.NewV4()
	if uuid_err != nil {
		return uuid_err
	}
	out_file := filepath.Join(retryFolder, file_id.String())
	file_err := ioutil.WriteFile(out_file, json, 0640)
	if file_err != nil {
		return file_err
	}
	return nil
}

type HandlerConfig struct {
	URL         string `mapstructure:"url"`
	RetryFolder string `mapstructure:"retry-folder"`
}

// handler provides the implementation of the alert.Handler interface for the Foo service.
type handler struct {
	s    *Service
	c    HandlerConfig
	diag Diagnostic
}

func (s *Service) DefaultHandlerConfig() HandlerConfig {
	return HandlerConfig{
		URL:         s.config().URL,
		RetryFolder: s.config().RetryFolder,
	}
}

func (s *Service) Handler(c HandlerConfig, ctx ...keyvalue.T) (alert.Handler, error) {
	// return a handler config populated with the default room from the service config.
	return &handler{
		s:    s,
		c:    c,
		diag: s.diag.WithContext(ctx...),
	}, nil
}

func (h *handler) Handle(event alert.Event) {
	if err := h.s.Alert(h.c.URL, h.c.RetryFolder, event); err != nil {
		h.diag.Error("failed to handle event to AlertManager", err)
	}
}

type testOptions struct {
	URL         string `json:"url"`
	RetryFolder string `json:"retry-folder"`
	Message     string `json:"message"`
}

func (s *Service) TestOptions() interface{} {
	c := s.config()
	return &testOptions{
		URL:     c.URL,
		Message: "test alertmanager message",
	}
}

func (s *Service) Test(o interface{}) error {
	options, ok := o.(*testOptions)
	if !ok {
		return fmt.Errorf("unexpected options type %T", options)
	}
	return s.Alert(options.URL, options.RetryFolder, alert.Event{})
}
