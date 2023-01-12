package director

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"open-match.dev/open-match/pkg/pb"

	agonesv1 "agones.dev/agones/pkg/apis/agones/v1"
	allocationv1 "agones.dev/agones/pkg/apis/allocation/v1"
	"agones.dev/agones/pkg/client/clientset/versioned"
	"agones.dev/agones/pkg/client/informers/externalversions"
	v1 "agones.dev/agones/pkg/client/informers/externalversions/agones/v1"
	"agones.dev/agones/pkg/util/runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	// The endpoint for the Open Match Backend service.
	OM_API_HOST = "open-match-backend.open-match.svc.cluster.local:50505"
	// The Host and Port for the Match Function service endpoint.
	MMF_API_HOST       = "pixo-matchfunction.default.svc.cluster.local" // Change to reflect the deployed service and port
	MMF_API_PORT int32 = 50502
)

// Variables for the logger and Agones Clientset
var (
	logger       = runtime.NewLoggerWithSource("main")
	namespace    = getEnv("FLEET_NAMESPACE", "default") // default
	fleetname    = getEnv("FLEET_NAME", "pixo-games")   //"pixo-games" // pixo-games
	agonesClient = getAgonesClient()
)

// get the environment variable or provide a default
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func createOMBackendClient() (pb.BackendServiceClient, func() error) {
	conn, err := grpc.Dial(OM_API_HOST, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(err)
	}
	return pb.NewBackendServiceClient(conn), conn.Close
}

// Set up our client which we will use to call the API
func getAgonesClient() *versioned.Clientset {
	// Create the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		logger.WithError(err).Fatal("Could not create in cluster config")
	}
	// Access to the Agones resources through the Agones Clientset
	agonesClient, err := versioned.NewForConfig(config)
	if err != nil {
		logger.WithError(err).Fatal("Could not create the agones api clientset")
	}
	return agonesClient
}

type InformerType struct {
	gameInformer v1.GameServerInformer
	podInformer  coreinformers.PodInformer
}

// GameServer informer and PodInformer
func getGameServerInfomer() (*InformerType, error) {
	// Create the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		logger.WithError(err).Fatal("Could not create in cluster config")
		return nil, err
	}
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.WithError(err).Fatal("Could not create the Kubernetes api Clientset")
		return nil, err
	}
	// Create InformerFactory which create the informer
	informerFactory := informers.NewSharedInformerFactory(kubeClient, time.Second*30)
	agonesInformerFactory := externalversions.NewSharedInformerFactory(agonesClient, time.Second*30)

	// Create Pod informer by informerFactory
	podInformer := informerFactory.Core().V1().Pods()

	// Create GameServer informer by informerFactory
	gameServers := agonesInformerFactory.Agones().V1().GameServers()

	c := &InformerType{
		gameInformer: gameServers,
		podInformer:  podInformer,
	}
	return c, nil
}

func createAgonesGameServerAllocation() *allocationv1.GameServerAllocation {
	return &allocationv1.GameServerAllocation{
		Spec: allocationv1.GameServerAllocationSpec{
			Required: allocationv1.GameServerSelector{
				LabelSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{agonesv1.FleetNameLabel: fleetname},
				},
			},
		},
	}
}

// Customize the backend.FetchMatches request, the default one will return all tickets in the statestore
func createOMFetchMatchesRequest(p *pb.MatchProfile) *pb.FetchMatchesRequest {
	return &pb.FetchMatchesRequest{
		// om-function:50502 -> the internal hostname & port number of the MMF service in our Kubernetes cluster
		Config: &pb.FunctionConfig{
			Host: MMF_API_HOST,
			Port: MMF_API_PORT,
			Type: pb.FunctionConfig_GRPC,
		},
		Profile: p,
	}
}

type GameServerIPPort struct {
	address string
	port    int32
}

// Get IP Address of an allocated game server
func getAllocatedGameServerInfo() (*GameServerIPPort, error) {
	listGameServer, err := agonesClient.AgonesV1().GameServers(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	n := rand.Intn(len(listGameServer.Items))
	gameserver := listGameServer.Items[n].Status

	c := &GameServerIPPort{
		address: gameserver.Address,
		port:    gameserver.Ports[0].Port,
	}
	return c, nil
}

// Return the number of ready game servers available to this fleet for allocation
func checkReadyReplicas() int32 {
	// Get a FleetInterface for this namespace
	fleetInterface := agonesClient.AgonesV1().Fleets(namespace)
	// Get our fleet
	fleet, err := fleetInterface.Get(context.Background(), fleetname, metav1.GetOptions{})
	if err != nil {
		logger.WithError(err).Info("Get fleet failed")
	}

	return fleet.Status.ReadyReplicas
}

func createOMAssignTicketRequest(match *pb.Match, address string, port int32) *pb.AssignTicketsRequest {
	tids := []string{}
	for _, t := range match.GetTickets() {
		tids = append(tids, t.GetId())
	}

	return &pb.AssignTicketsRequest{
		Assignments: []*pb.AssignmentGroup{
			{
				TicketIds: tids,
				Assignment: &pb.Assignment{
					Connection: fmt.Sprintf("%s:%d", address, port),
				},
			},
		},
	}
}

func fetch(bc pb.BackendServiceClient, p *pb.MatchProfile) ([]*pb.Match, error) {
	// this needs to be modified to fetch the correct profile
	stream, err := bc.FetchMatches(context.Background(), createOMFetchMatchesRequest(p))
	if err != nil {
		return nil, err
	}
	var result []*pb.Match
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, err
		}

		result = append(result, resp.GetMatch())
	}

	return result, nil
}

// Move a replica from ready to allocated and return the GameServerStatus
func allocate() (*GameServerIPPort, error) {
	ctx := context.Background()

	// Find out how many ready replicas the fleet has - we need at least one
	gsa, err := agonesClient.AllocationV1().GameServerAllocations(namespace).Create(ctx,
		createAgonesGameServerAllocation(), metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}

	if gsa.Status.State != allocationv1.GameServerAllocationAllocated {
		return nil, errors.New("failed to allocate game server")
	}

	// Log the GameServer.Staus of the new allocation, then return those values
	logger.Infof("New GameServer allocated: %v", gsa.Status.State)

	address, port := gsa.Status.Address, gsa.Status.Ports[0].Port

	return &GameServerIPPort{
		address: address,
		port:    port,
	}, nil
}

func assign(bc pb.BackendServiceClient, matches []*pb.Match, gameSrvInfo *GameServerIPPort) error {
	for _, match := range matches {

		if _, err := bc.AssignTickets(context.Background(), createOMAssignTicketRequest(match, gameSrvInfo.address, gameSrvInfo.port)); err != nil {
			return fmt.Errorf("AssignTickets failed for match %v, got %w", match.GetMatchId(), err)
		}

		conn := fmt.Sprintf("%s:%d", gameSrvInfo.address, gameSrvInfo.port)
		logger.Infof("Assigned server %s to match %s", conn, match.GetMatchId())
	}

	return nil
}

func Run() {
	// Connect to Open Match Backend.
	bc, omCloser := createOMBackendClient()
	defer omCloser()

	rand.Seed(time.Now().Unix())
	// Generate the profiles to fetch matches for.
	profiles := generateProfiles()

	for range time.Tick(time.Second * 2) {
		// Fetch matches for each profile and make random assignments for Tickets in
		// the matches returned.
		var wg sync.WaitGroup
		for _, p := range profiles {
			wg.Add(1)
			go func(wg *sync.WaitGroup, p *pb.MatchProfile) {
				defer wg.Done()

				matches, err := fetch(bc, p)
				if err != nil {
					logger.WithError(err).Info("Failed to fetch matches")
					return
				}
				// if not matches then continue
				if len(matches) > 0 {
					readyReplicas := checkReadyReplicas()

					// Log and return an error if there are no ready replicas
					if readyReplicas < 1 {
						g, err := getAllocatedGameServerInfo()
						if err != nil {
							logger.WithError(err).Error("Failed to get game server info")
							return
						}
						err = assign(bc, matches, g)
						if err != nil {
							logger.WithError(err).Error("Failed to assign servers to matches")
						}
					} else {
						// allocate the server
						allocatedata, err := allocate()
						if err != nil {
							logger.WithError(err).Info("Failed to assign servers to matches")
						}
						assign(bc, matches, allocatedata)
					}
				}
			}(&wg, p)
		}
		wg.Wait()
	}
}
