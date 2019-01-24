package routes

import (
	"context"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	bgp "github.com/magicsong/porter/pkg/bgp/serverd"
	api "github.com/osrg/gobgp/api"
)

func toAPIPath(ip string, prefix uint32, nexthop string) *api.Path {
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

func isRouteAdded(ip string, prefix uint32, nexthop string) bool {
	lookup := &api.TableLookupPrefix{
		Prefix: ip,
	}
	listPathRequest := &api.ListPathRequest{
		TableType: api.TableType_GLOBAL,
		Family:    &api.Family{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST},
		Prefixes:  []*api.TableLookupPrefix{lookup},
	}
	var result bool
	fn := func(d *api.Destination) {
		if len(d.Paths) > 0 {
			result = true
		}
	}
	err := bgp.GetServer().ListPath(context.Background(), listPathRequest, fn)
	if err != nil {
		panic(err)
	}
	return result
}
func AddRoute(ip string, prefix uint32, nexthop string) error {
	s := bgp.GetServer()
	if !isRouteAdded(ip, prefix, nexthop) {
		return nil
	}
	apipath := toAPIPath(ip, prefix, nexthop)
	_, err := s.AddPath(context.Background(), &api.AddPathRequest{
		Path: apipath,
	})
	return err
}

func deleteRoute(ip string, prefix uint32, nexthop string) error {
	s := bgp.GetServer()
	apipath := toAPIPath(ip, prefix, nexthop)
	return s.DeletePath(context.Background(), &api.DeletePathRequest{
		Path: apipath,
	})
}
