package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/measurements"
)

type measurementEnvelope struct {
	MeasurementName string          `json:"measurement_name"`
	Parameters      json.RawMessage `json:"parameters"`
}

type instrumentTarget struct {
	ID      string `json:"id"`
	Channel int    `json:"channel"`
}

type getVoltageParams struct {
	Getter instrumentTarget `json:"getter"`
}

type setVoltageParams struct {
	Setter     instrumentTarget `json:"setter"`
	SetVoltage float64          `json:"setVoltage"`
}

type measureCurrentParams struct {
	Getter instrumentTarget `json:"getter"`
	Source instrumentTarget `json:"source"`
}

type dummyBackend struct {
	nc              *nats.Conn
	voltageState    map[string]float64
	preampGain      float64
	preampOffsetAmp float64
	subscription    *nats.Subscription
	startupState    float64
}

type harnessFlags struct {
	NATSURL           string
	ClientName        string
	Timeout           time.Duration
	StartEmbeddedNATS bool
	StartLocalHub     bool
	StartDummyBackend bool
	InitialVoltage    float64
	SetVoltage        float64
	PreampGain        float64
	PreampOffsetAmp   float64
}

type localHub struct {
	logger             *logging.Logger
	measurementManager *measurements.Manager
	measureHandler     *handlers.MeasureCommandHandler
	nc                 *nats.Conn
	tempDir            string
}

type busyState struct{}

func (busyState) SetIsBusy(bool) {}

func main() {
	flags := parseFlags()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var embedded *natsserver.Server
	if flags.StartEmbeddedNATS {
		server, url, err := startEmbeddedNATSServer()
		if err != nil {
			log.Fatalf("start embedded nats: %v", err)
		}
		embedded = server
		flags.NATSURL = url
		defer embedded.Shutdown()
		log.Printf("embedded NATS running at %s", url)
	}

	nc, err := nats.Connect(flags.NATSURL)
	if err != nil {
		log.Fatalf("connect to NATS %s: %v", flags.NATSURL, err)
	}
	defer nc.Drain()

	var hub *localHub
	if flags.StartLocalHub {
		hub, err = startLocalHub(nc, flags.NATSURL)
		if err != nil {
			log.Fatalf("start local hub edge: %v", err)
		}
		defer hub.close()
		log.Printf("local hub edge bridge enabled")
	}

	var backend *dummyBackend
	if flags.StartDummyBackend {
		backend, err = startDummyBackend(nc, flags)
		if err != nil {
			log.Fatalf("start dummy backend: %v", err)
		}
		defer backend.close()
		log.Printf("dummy backend enabled")
	}

	if err := runScenario(ctx, nc, flags); err != nil {
		log.Fatalf("scenario failed: %v", err)
	}
}

func parseFlags() harnessFlags {
	var flags harnessFlags
	var timeout time.Duration
	flag.StringVar(&flags.NATSURL, "nats-url", nats.DefaultURL, "NATS URL used by the hub runtime")
	flag.StringVar(&flags.ClientName, "client-name", "external-harness", "suffix used for MEASURE_COMMAND.external.<name>")
	flag.DurationVar(&timeout, "timeout", 5*time.Second, "timeout for each request/response round-trip")
	flag.BoolVar(&flags.StartEmbeddedNATS, "start-embedded-nats", false, "start an embedded NATS server for fully local smoke tests")
	flag.BoolVar(&flags.StartLocalHub, "start-local-hub", false, "start the hub edge measure-command bridge in-process")
	flag.BoolVar(&flags.StartDummyBackend, "start-dummy-backend", false, "start a dummy PROCESS_REQUEST -> UPLOAD_DATA backend in-process")
	flag.Float64Var(&flags.InitialVoltage, "initial-voltage", 0.125, "initial voltage exposed by the dummy voltage source")
	flag.Float64Var(&flags.SetVoltage, "set-voltage", 0.250, "target voltage used in the set_voltage step")
	flag.Float64Var(&flags.PreampGain, "preamp-gain", 8e-9, "dummy current preamp transfer function in A/V")
	flag.Float64Var(&flags.PreampOffsetAmp, "preamp-offset-amp", 1e-10, "dummy current preamp offset in amperes")
	flag.Parse()
	flags.Timeout = timeout
	return flags
}

func runScenario(ctx context.Context, nc *nats.Conn, flags harnessFlags) error {
	responseSubject := fmt.Sprintf("MEASURE_RESPONSE.external.%s", flags.ClientName)
	responseSub, err := nc.SubscribeSync(responseSubject)
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", responseSubject, err)
	}
	defer responseSub.Unsubscribe()

	requests := []struct {
		label    string
		envelope measurementEnvelope
	}{
		{
			label: "get voltage",
			envelope: mustEnvelope("get_voltage", getVoltageParams{
				Getter: instrumentTarget{ID: "VSRC1", Channel: 0},
			}),
		},
		{
			label: "set voltage",
			envelope: mustEnvelope("set_voltage", setVoltageParams{
				Setter:     instrumentTarget{ID: "VSRC1", Channel: 0},
				SetVoltage: flags.SetVoltage,
			}),
		},
		{
			label: "read current",
			envelope: mustEnvelope("measure_current", measureCurrentParams{
				Getter: instrumentTarget{ID: "PREAMP1", Channel: 0},
				Source: instrumentTarget{ID: "VSRC1", Channel: 0},
			}),
		},
	}

	for index, request := range requests {
		hash := time.Now().UnixNano() + int64(index)
		response, err := sendMeasureCommand(ctx, nc, responseSub, flags, hash, request.envelope)
		if err != nil {
			return fmt.Errorf("%s: %w", request.label, err)
		}
		formatted, err := prettyResponse(response.Response)
		if err != nil {
			return fmt.Errorf("%s: pretty print response: %w", request.label, err)
		}
		fmt.Printf("[%d/%d] %s\n%s\n", index+1, len(requests), request.label, formatted)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func sendMeasureCommand(
	ctx context.Context,
	nc *nats.Conn,
	responseSub *nats.Subscription,
	flags harnessFlags,
	hash int64,
	envelope measurementEnvelope,
) (*api.MeasureResponse, error) {
	requestBytes, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal envelope: %w", err)
	}

	command := api.MeasureCommand{
		Request:   string(requestBytes),
		Timestamp: time.Now().UnixMicro(),
		Hash:      hash,
	}
	commandBytes, err := json.Marshal(command)
	if err != nil {
		return nil, fmt.Errorf("marshal measure command: %w", err)
	}

	commandSubject := fmt.Sprintf("MEASURE_COMMAND.external.%s", flags.ClientName)
	if err := nc.Publish(commandSubject, commandBytes); err != nil {
		return nil, fmt.Errorf("publish %s: %w", commandSubject, err)
	}
	if err := nc.Flush(); err != nil {
		return nil, fmt.Errorf("flush measure command: %w", err)
	}

	deadline := time.Now().Add(flags.Timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, fmt.Errorf("timeout waiting for MEASURE_RESPONSE hash=%d", hash)
		}
		msg, err := responseSub.NextMsg(remaining)
		if err != nil {
			return nil, fmt.Errorf("wait for MEASURE_RESPONSE: %w", err)
		}
		var response api.MeasureResponse
		if err := json.Unmarshal(msg.Data, &response); err != nil {
			return nil, fmt.Errorf("unmarshal MEASURE_RESPONSE: %w", err)
		}
		if response.Hash != hash {
			continue
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			return &response, nil
		}
	}
}

func mustEnvelope(name string, params any) measurementEnvelope {
	data, err := json.Marshal(params)
	if err != nil {
		panic(err)
	}
	return measurementEnvelope{MeasurementName: name, Parameters: data}
}

func prettyResponse(raw string) (string, error) {
	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return "  raw: " + raw, nil
	}
	formatted, err := json.MarshalIndent(decoded, "  ", "  ")
	if err != nil {
		return "", err
	}
	return string(formatted), nil
}

func startEmbeddedNATSServer() (*natsserver.Server, string, error) {
	port, err := freeTCPPort()
	if err != nil {
		return nil, "", err
	}
	options := &natsserver.Options{Host: "127.0.0.1", Port: port}
	server, err := natsserver.NewServer(options)
	if err != nil {
		return nil, "", err
	}
	go server.Start()
	if !server.ReadyForConnections(5 * time.Second) {
		server.Shutdown()
		return nil, "", errors.New("embedded NATS server did not become ready")
	}
	return server, fmt.Sprintf("nats://127.0.0.1:%d", port), nil
}

func freeTCPPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("unexpected listener address type")
	}
	return addr.Port, nil
}

func startLocalHub(nc *nats.Conn, natsURL string) (*localHub, error) {
	tempDir, err := os.MkdirTemp("", "falcon-runtime-harness-*")
	if err != nil {
		return nil, err
	}
	logger, err := logging.NewLogger(filepath.Join(tempDir, "logs"))
	if err != nil {
		return nil, err
	}
	measurementManager, err := measurements.NewManager(
		filepath.Join(tempDir, "data"),
		filepath.Join(tempDir, "harness.db"),
	)
	if err != nil {
		return nil, err
	}
	cfg := &config.Config{DeviceConfig: &config.DeviceConfig{}, WireMap: &config.WireMap{}}
	instrumentHandler, err := instrument.NewHandler(logger, natsURL, nc, cfg, "python3")
	if err != nil {
		return nil, err
	}
	injectDummyInstruments(instrumentHandler)

	measureHandler := handlers.NewMeasureCommandHandler(
		logger,
		measurementManager,
		instrumentHandler,
		busyState{},
	)
	if err := measureHandler.Subscribe(nc); err != nil {
		return nil, err
	}

	return &localHub{
		logger:             logger,
		measurementManager: measurementManager,
		measureHandler:     measureHandler,
		nc:                 nc,
		tempDir:            tempDir,
	}, nil
}

func (h *localHub) close() {
	if h.measureHandler != nil {
		_ = h.measureHandler.Unsubscribe()
	}
	if h.measurementManager != nil {
		h.measurementManager.Close()
	}
	if h.logger != nil {
		h.logger.Close()
	}
	if h.tempDir != "" {
		_ = os.RemoveAll(h.tempDir)
	}
}

func injectDummyInstruments(handler *instrument.Handler) {
	handler.Instruments = map[instrument.Name]*instrument.InstrumentProcess{
		"VSRC1": {
			Name:        "VSRC1",
			Initialized: true,
			Ports: map[instrument.PropertyName]map[instrument.Index]instrument.JsonPort{
				"knobs": {"0": createKnobJSON("P1")},
			},
			Configuration: map[instrument.PropertyName]map[instrument.Index]instrument.PortConfiguration{
				"knobs": {
					"0": {"bounds": []float64{-10, 10}, "unit": "V"},
				},
			},
		},
		"PREAMP1": {
			Name:        "PREAMP1",
			Initialized: true,
			Ports: map[instrument.PropertyName]map[instrument.Index]instrument.JsonPort{
				"meters": {"0": createMeterJSON("I_O1")},
			},
			Configuration: map[instrument.PropertyName]map[instrument.Index]instrument.PortConfiguration{
				"meters": {
					"0": {"unit": "A"},
				},
			},
		},
	}
}

func createKnobJSON(connection config.InstrumentConnection) instrument.JsonPort {
	return mustPortJSON(instrument.PortObject{
		Class:       "Knob",
		Module:      "falcon_core.instrument_interfaces.names.knob",
		DefaultName: "VSRC1__##__voltage_state__##__0",
		PseudoName: instrument.PsuedoName{
			Class:  instrument.PlungerGate,
			Module: "falcon_core.physics.device_structures.plunger_gate",
			Name:   connection,
		},
		InstrumentType: "DAC",
		Units:          map[string]any{"unit": "V"},
		Description:    "Dummy voltage source channel",
	})
}

func createMeterJSON(connection config.InstrumentConnection) instrument.JsonPort {
	return mustPortJSON(instrument.PortObject{
		Class:       "Meter",
		Module:      "falcon_core.instrument_interfaces.names.meter",
		DefaultName: "PREAMP1__##__current_state__##__0",
		PseudoName: instrument.PsuedoName{
			Class:  instrument.Ohmic,
			Module: "falcon_core.physics.device_structures.ohmic",
			Name:   connection,
		},
		InstrumentType: "CURRENT_PREAMP",
		Units:          map[string]any{"unit": "A"},
		Description:    "Dummy current pre-amplifier channel",
	})
}

func mustPortJSON(port instrument.PortObject) instrument.JsonPort {
	data, err := json.Marshal(port)
	if err != nil {
		panic(err)
	}
	return instrument.JsonPort(data)
}

func startDummyBackend(nc *nats.Conn, flags harnessFlags) (*dummyBackend, error) {
	backend := &dummyBackend{
		nc: nc,
		voltageState: map[string]float64{
			stateKey("VSRC1", 0): flags.InitialVoltage,
		},
		preampGain:      flags.PreampGain,
		preampOffsetAmp: flags.PreampOffsetAmp,
		startupState:    flags.InitialVoltage,
	}
	sub, err := nc.Subscribe("PROCESS_REQUEST", backend.handleProcessRequest)
	if err != nil {
		return nil, err
	}
	backend.subscription = sub
	return backend, nil
}

func (b *dummyBackend) close() {
	if b.subscription != nil {
		_ = b.subscription.Unsubscribe()
	}
}

func (b *dummyBackend) handleProcessRequest(msg *nats.Msg) {
	var request api.ProcessRequest
	if err := json.Unmarshal(msg.Data, &request); err != nil {
		log.Printf("dummy backend: invalid PROCESS_REQUEST: %v", err)
		return
	}

	var envelope measurementEnvelope
	if err := json.Unmarshal([]byte(request.Request), &envelope); err != nil {
		log.Printf("dummy backend: invalid request envelope: %v", err)
		return
	}

	response, err := b.execute(envelope)
	if err != nil {
		response = map[string]any{
			"ok":    false,
			"error": err.Error(),
		}
	}
	responseBytes, err := json.Marshal(response)
	if err != nil {
		log.Printf("dummy backend: marshal response: %v", err)
		return
	}

	upload := api.UploadData{
		Data:      string(responseBytes),
		Timestamp: time.Now().UnixMicro(),
		ProcessId: request.ProcessId,
	}
	uploadBytes, err := json.Marshal(upload)
	if err != nil {
		log.Printf("dummy backend: marshal upload data: %v", err)
		return
	}
	if err := b.nc.Publish("UPLOAD_DATA", uploadBytes); err != nil {
		log.Printf("dummy backend: publish upload data: %v", err)
	}
}

func (b *dummyBackend) execute(envelope measurementEnvelope) (map[string]any, error) {
	switch envelope.MeasurementName {
	case "get_voltage":
		var params getVoltageParams
		if err := json.Unmarshal(envelope.Parameters, &params); err != nil {
			return nil, err
		}
		value := b.voltageState[stateKey(params.Getter.ID, params.Getter.Channel)]
		return map[string]any{
			"ok":         true,
			"instrument": params.Getter.ID,
			"channel":    params.Getter.Channel,
			"verb":       "GET_VOLTAGE",
			"type":       "number",
			"unit":       "V",
			"value":      round(value, 12),
		}, nil
	case "set_voltage":
		var params setVoltageParams
		if err := json.Unmarshal(envelope.Parameters, &params); err != nil {
			return nil, err
		}
		b.voltageState[stateKey(params.Setter.ID, params.Setter.Channel)] = params.SetVoltage
		return map[string]any{
			"ok":         true,
			"instrument": params.Setter.ID,
			"channel":    params.Setter.Channel,
			"verb":       "SET_VOLTAGE",
			"type":       "ack",
			"unit":       "V",
			"value":      round(params.SetVoltage, 12),
		}, nil
	case "measure_current":
		var params measureCurrentParams
		if err := json.Unmarshal(envelope.Parameters, &params); err != nil {
			return nil, err
		}
		voltage := b.voltageState[stateKey(params.Source.ID, params.Source.Channel)]
		current := b.preampOffsetAmp + b.preampGain*voltage
		return map[string]any{
			"ok":               true,
			"instrument":       params.Getter.ID,
			"channel":          params.Getter.Channel,
			"verb":             "MEASURE_CURRENT",
			"type":             "number",
			"unit":             "A",
			"sourceInstrument": params.Source.ID,
			"sourceChannel":    params.Source.Channel,
			"voltage":          round(voltage, 12),
			"value":            round(current, 15),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported measurement_name %q", envelope.MeasurementName)
	}
}

func stateKey(id string, channel int) string {
	return fmt.Sprintf("%s:%d", id, channel)
}

func round(value float64, precision int) float64 {
	scale := math.Pow10(precision)
	return math.Round(value*scale) / scale
}
