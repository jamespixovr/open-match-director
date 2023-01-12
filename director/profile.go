package director

import (
	"fmt"
	"strconv"

	"google.golang.org/protobuf/types/known/anypb"
	"open-match.dev/open-match/pkg/pb"
)

type OrgModule struct {
	ModuleId  int
	OrgId     int
	FleetName string
	Namespace string
}

type GameProfile struct {
	profile pb.MatchProfile
	org     OrgModule
}

// generateGameProfile generates test profiles for the matchmaker101 tutorial.
func generateGameProfile() []*GameProfile {
	var gameprofiles []*GameProfile

	orgModules := []OrgModule{
		{OrgId: 5, ModuleId: 4, FleetName: "simple-game-server", Namespace: "default"},
		{OrgId: 2, ModuleId: 3, FleetName: "pixo-game-server", Namespace: "default"},
	}

	var pools []*pb.Pool

	for _, orgModule := range orgModules {
		pools = []*pb.Pool{}
		pools = append(pools, &pb.Pool{
			Name: fmt.Sprintf("pool_%s_%s", strconv.Itoa(orgModule.OrgId), strconv.Itoa(orgModule.ModuleId)),
			StringEqualsFilters: []*pb.StringEqualsFilter{
				{
					StringArg: "attributes.moduleId",
					Value:     strconv.Itoa(orgModule.ModuleId),
				},
				{
					StringArg: "attributes.orgId",
					Value:     strconv.Itoa(orgModule.OrgId),
				},
			},
		})

		c := &GameProfile{
			profile: pb.MatchProfile{
				Name:       "profile_" + strconv.Itoa(orgModule.OrgId) + "_" + strconv.Itoa(orgModule.ModuleId),
				Pools:      pools,
				Extensions: map[string]*anypb.Any{},
			},
			org: orgModule,
		}

		gameprofiles = append(gameprofiles, c)
	}
	return gameprofiles
}
