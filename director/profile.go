package director

import (
	"fmt"
	"strconv"

	"google.golang.org/protobuf/types/known/anypb"
	"open-match.dev/open-match/pkg/pb"
)

type OrgModule struct {
	ModuleId int
	OrgId    int
}

// generateProfiles generates test profiles for the matchmaker101 tutorial.
func generateProfiles() []*pb.MatchProfile {
	var profiles []*pb.MatchProfile

	// TODO will need a connection to db to grab org/module combinations
	orgModules := []OrgModule{{OrgId: 5, ModuleId: 4}, {OrgId: 2, ModuleId: 3}}
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

		profiles = append(profiles, &pb.MatchProfile{
			Name:       "profile_" + strconv.Itoa(orgModule.OrgId) + "_" + strconv.Itoa(orgModule.ModuleId),
			Pools:      pools,
			Extensions: map[string]*anypb.Any{},
		})
	}
	return profiles
}
