package routes

import (
	"context"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	bgp "github.com/magicsong/porter/pkg/bgp/serverd"
	api "github.com/osrg/gobgp/api"
)

func toAPIPath(ip string, prefix int, nexthop string) *api.Path {
	nlri, _ := ptypes.MarshalAny(&api.IPAddressPrefix{
		Prefix:    ip,
		PrefixLen: prefix,
	})
	a1, _ := ptypes.MarshalAny(&api.OriginAttribute{
		Origin: 0,
	})
	a2, _ := ptypes.MarshalAny(&api.NextHopAttribute{
		NextHop: nexthop,
	})
	attrs := []*any.Any{a1, a2}
	return &api.Path{
		Family: &api.Family{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST},
		Nlri:   nlri,
		Pattrs: attrs,
	}
}
func AddRoute(ip string, prefix int, nexthop string) error {
	s := bgp.GetServer()
	apipath := toAPIPath(ip, prefix, nexthop)
	_, err := s.AddPath(context.Background(), &api.AddPathRequest{
		Path: apipath,
	})
	return err
}

func deleteRoute(ip string, prefix int, nexthop string) error {
	s := bgp.GetServer()
	apipath := toAPIPath(ip, prefix, nexthop)
	_, err := s.DeletePath(context.Background(), &api.DeletePathRequest{
		Path: apipath,
	})
	return err
}
