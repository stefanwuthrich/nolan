package cmd

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/bdkiran/nolan/nolan"
	"github.com/bdkiran/nolan/nolan/config"
	"github.com/spf13/cobra"
	gracefully "github.com/tj/go-gracefully"
	"github.com/uber/jaeger-lib/metrics"

	"github.com/hashicorp/memberlist"
	"github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	jaegerlog "github.com/uber/jaeger-client-go/log"
)

var (
	brokerCmd = &cobra.Command{
		Use:   "broker",
		Short: "Manage broker",
		Long:  `Everything nolan related with broker. Manage here`,
		Run:   run,
	}

	brokerCfg  = config.DefaultConfig()
	nodeNumber int32
)

func init() {
	//brokerCmd := &cobra.Command{Use: "broker", Short: "Run a Jocko broker", Run: run, Args: cobra.NoArgs}
	brokerCmd.Flags().StringVar(&brokerCfg.RaftAddr, "raft-addr", "127.0.0.1:9093", "Address for Raft to bind and advertise on")
	brokerCmd.Flags().StringVar(&brokerCfg.DataDir, "data-dir", "/tmp/jocko", "A comma separated list of directories under which to store log files")
	brokerCmd.Flags().StringVar(&brokerCfg.Addr, "broker-addr", "0.0.0.0:9092", "Address for broker to bind on")
	brokerCmd.Flags().Var(newMemberlistConfigValue(brokerCfg.SerfLANConfig.MemberlistConfig, "0.0.0.0:9094"), "serf-addr", "Address for Serf to bind on")
	brokerCmd.Flags().BoolVar(&brokerCfg.Bootstrap, "bootstrap", false, "Initial cluster bootstrap (dangerous!)")
	brokerCmd.Flags().IntVar(&brokerCfg.BootstrapExpect, "bootstrap-expect", 0, "Expected number of nodes in cluster")
	brokerCmd.Flags().StringSliceVar(&brokerCfg.StartJoinAddrsLAN, "join", nil, "Address of an broker serf to join at start time. Can be specified multiple times.")
	brokerCmd.Flags().StringSliceVar(&brokerCfg.StartJoinAddrsWAN, "join-wan", nil, "Address of an broker serf to join -wan at start time. Can be specified multiple times.")
	brokerCmd.Flags().Int32Var(&brokerCfg.ID, "id", 0, "Broker ID")

	createTestBrokerCmd := &cobra.Command{Use: "test", Short: "Create a test broker", Run: createTestBroker, Args: cobra.NoArgs}
	brokerCmd.AddCommand(createTestBrokerCmd)
	//cli.AddCommand(brokerCmd)
}

func run(cmd *cobra.Command, args []string) {
	var err error

	log.SetPrefix(fmt.Sprintf("jocko: node id: %d: ", brokerCfg.ID))

	cfg := jaegercfg.Configuration{
		Sampler: &jaegercfg.SamplerConfig{
			Type:  jaeger.SamplerTypeConst,
			Param: 1,
		},
		Reporter: &jaegercfg.ReporterConfig{
			LogSpans: true,
		},
	}

	jLogger := jaegerlog.StdLogger
	jMetricsFactory := metrics.NullFactory

	tracer, closer, err := cfg.New(
		"jocko",
		jaegercfg.Logger(jLogger),
		jaegercfg.Metrics(jMetricsFactory),
	)
	if err != nil {
		panic(err)
	}

	broker, err := nolan.NewBroker(brokerCfg, tracer)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error starting broker: %v\n", err)
		os.Exit(1)
	}

	srv := nolan.NewServer(brokerCfg, broker, nil, tracer, closer.Close)
	if err := srv.Start(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "error starting server: %v\n", err)
		os.Exit(1)
	}

	defer srv.Shutdown()

	gracefully.Timeout = 10 * time.Second
	gracefully.Shutdown()

	if err := broker.Shutdown(); err != nil {
		fmt.Fprintf(os.Stderr, "error shutting down store: %v\n", err)
		os.Exit(1)
	}
}

//Used to create a broker similar to that in test...
func createTestBroker(cmd *cobra.Command, args []string) {
	fmt.Println("Starting test broker...")

	nodeID := atomic.AddInt32(&nodeNumber, 1)

	cfg := jaegercfg.Configuration{
		Sampler: &jaegercfg.SamplerConfig{
			Type:  jaeger.SamplerTypeConst,
			Param: 1,
		},
		Reporter: &jaegercfg.ReporterConfig{
			LogSpans: true,
		},
	}

	// jLogger := jaegerlog.StdLogger
	jMetricsFactory := metrics.NullFactory

	tracer, closer, err := cfg.New(
		"nolan",
		// jaegercfg.Logger(jLogger),
		jaegercfg.Metrics(jMetricsFactory),
	)
	if err != nil {
		panic(err)
	}

	tmpDir, err := ioutil.TempDir("", fmt.Sprintf("jocko-test-server-%d", nodeID))
	if err != nil {
		panic(err)
	}

	config := config.DefaultConfig()
	config.ID = nodeID
	config.NodeName = fmt.Sprintf("%s-node-%d", "test", nodeID)
	config.DataDir = tmpDir
	config.Addr = fmt.Sprintf("%s:%d", "127.0.0.1", 9092)
	config.RaftAddr = fmt.Sprintf("%s:%d", "127.0.0.1", 9093)
	config.SerfLANConfig.MemberlistConfig.BindAddr = "127.0.0.1"
	config.SerfLANConfig.MemberlistConfig.BindPort = 9094
	config.LeaveDrainTime = 1 * time.Millisecond
	config.ReconcileInterval = 300 * time.Millisecond

	// Tighten the Serf timing
	config.SerfLANConfig.MemberlistConfig.BindAddr = "127.0.0.1"
	config.SerfLANConfig.MemberlistConfig.SuspicionMult = 2
	config.SerfLANConfig.MemberlistConfig.RetransmitMult = 2
	config.SerfLANConfig.MemberlistConfig.ProbeTimeout = 50 * time.Millisecond
	config.SerfLANConfig.MemberlistConfig.ProbeInterval = 100 * time.Millisecond
	config.SerfLANConfig.MemberlistConfig.GossipInterval = 100 * time.Millisecond

	// Tighten the Raft timing
	config.RaftConfig.LeaderLeaseTimeout = 100 * time.Millisecond
	config.RaftConfig.HeartbeatTimeout = 200 * time.Millisecond
	config.RaftConfig.ElectionTimeout = 200 * time.Millisecond

	config.Bootstrap = true
	config.BootstrapExpect = 1
	config.StartAsLeader = true

	broker, err := nolan.NewBroker(config, tracer)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error starting broker: %v\n", err)
		os.Exit(1)
	}

	srv := nolan.NewServer(config, broker, nil, tracer, closer.Close)
	if err := srv.Start(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "error starting server: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("--------Broker and server located at....----------")
	fmt.Println(config.Addr)
	fmt.Println(srv.Addr().String())

	_, err = nolan.Dial("tcp", srv.Addr().String())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error connecting to broker: %v\n", err)
		os.Exit(1)
	}

	defer srv.Shutdown()

	gracefully.Timeout = 10 * time.Second
	gracefully.Shutdown()

	if err := broker.Shutdown(); err != nil {
		fmt.Fprintf(os.Stderr, "error shutting down store: %v\n", err)
		os.Exit(1)
	}
}

type memberlistConfigValue memberlist.Config

func newMemberlistConfigValue(p *memberlist.Config, val string) (m *memberlistConfigValue) {
	m = (*memberlistConfigValue)(p)
	m.Set(val)
	return
}

func (v *memberlistConfigValue) Set(s string) error {
	bindIP, bindPort, err := net.SplitHostPort(s)
	if err != nil {
		return err
	}
	v.BindAddr = bindIP
	v.BindPort, err = strconv.Atoi(bindPort)
	if err != nil {
		return err
	}
	return nil
}

func (v *memberlistConfigValue) Type() string {
	return "string"
}

func (v *memberlistConfigValue) String() string {
	return fmt.Sprintf("%s:%d", v.BindAddr, v.BindPort)
}
