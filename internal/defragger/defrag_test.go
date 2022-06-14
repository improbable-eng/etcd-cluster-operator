package defragger

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/improbable-eng/etcd-cluster-operator/internal/etcd"
	"github.com/stretchr/testify/assert"
)

type fakeUsedSpaceChecker struct {
	usedSpacePercentage map[string]int
	usedSpaceError      map[string]error
}

func (f *fakeUsedSpaceChecker) UsedSpacePercentage(_ context.Context, member etcd.Member) (int, error) {
	return f.usedSpacePercentage[member.ID], f.usedSpaceError[member.ID]
}

type fakeDefragger struct {
	defragError      map[string]error
	defraggedCallMap map[string]int
}

func (f *fakeDefragger) Defragment(_ context.Context, member etcd.Member) error {
	f.defraggedCallMap[member.ID]++
	return f.defragError[member.ID]
}

func Test_DefragIfNecessary(t *testing.T) {

	tests := []struct {
		name                string
		usedSpacePercentage map[string]int
		usedSpaceError      map[string]error
		defragError         map[string]error
		members             []etcd.Member
		defragThreshold     uint

		wantErr          error
		wantDefragCalled map[string]int
	}{
		{
			name: "should not defrag",

			members: []etcd.Member{
				{ID: "member1"},
				{ID: "member2"},
			},
			defragThreshold:     60,
			usedSpacePercentage: map[string]int{"member1": 5, "member2": 45},

			wantErr:          nil,
			wantDefragCalled: map[string]int{},
		},
		{
			name: "should defrag one member",

			members: []etcd.Member{
				{ID: "member1"},
				{ID: "member2"},
			},
			defragThreshold:     90,
			usedSpacePercentage: map[string]int{"member1": 85, "member2": 95},

			wantErr:          nil,
			wantDefragCalled: map[string]int{"member2": 1},
		},
		{
			name: "should defrag both members",

			members: []etcd.Member{
				{ID: "member1"},
				{ID: "member2"},
			},
			defragThreshold:     80,
			usedSpacePercentage: map[string]int{"member1": 81, "member2": 95},

			wantErr:          nil,
			wantDefragCalled: map[string]int{"member1": 1, "member2": 1},
		},
		{
			name: "used space returns an error",

			members: []etcd.Member{
				{ID: "member1"},
				{ID: "member2"},
			},
			defragThreshold:     80,
			usedSpacePercentage: map[string]int{"member2": 95},
			usedSpaceError: map[string]error{
				"member1": errors.New("failed to get used space"),
			},

			wantErr:          errors.New("failed to get used space"),
			wantDefragCalled: map[string]int{"member2": 1},
		},
		{
			name: "defrag returns an error- both members should be defragged",

			members: []etcd.Member{
				{ID: "member1"},
				{ID: "member2"},
			},
			defragThreshold:     80,
			usedSpacePercentage: map[string]int{"member1": 81, "member2": 95},
			defragError: map[string]error{
				"member1": errors.New("failed to defrag"),
			},

			wantErr:          errors.New("failed to defrag"),
			wantDefragCalled: map[string]int{"member1": 1, "member2": 1},
		},
	}

	for _, tt := range tests {
		var tt = tt
		t.Run(tt.name, func(t *testing.T) {
			u := &fakeUsedSpaceChecker{usedSpacePercentage: tt.usedSpacePercentage, usedSpaceError: tt.usedSpaceError}
			d := &fakeDefragger{defragError: tt.defragError, defraggedCallMap: make(map[string]int)}

			err := DefragIfNecessary(context.Background(), tt.defragThreshold, tt.members, u, d, logr.Discard())
			assert.Equal(t, tt.wantErr, err)
			assert.Equal(t, tt.wantDefragCalled, d.defraggedCallMap)

		})
	}
}

func Test_Defrag(t *testing.T) {

	tests := []struct {
		name             string
		defragError      map[string]error
		members          []etcd.Member
		wantErr          error
		wantDefragCalled map[string]int
	}{
		{
			name: "should defrag both members",
			members: []etcd.Member{
				{ID: "member1"},
				{ID: "member2"},
			},
			wantErr:          nil,
			wantDefragCalled: map[string]int{"member1": 1, "member2": 1},
		},
		{
			name: "defrag returns an error",

			members: []etcd.Member{
				{ID: "member1"},
				{ID: "member2"},
			},
			defragError: map[string]error{
				"member1": errors.New("member1: failed to defrag"),
			},

			wantErr:          errors.New("member1: failed to defrag"),
			wantDefragCalled: map[string]int{"member1": 1, "member2": 1},
		},
	}

	for _, tt := range tests {
		var tt = tt
		t.Run(tt.name, func(t *testing.T) {
			d := &fakeDefragger{defragError: tt.defragError, defraggedCallMap: make(map[string]int)}

			err := Defrag(context.Background(), tt.members, d, logr.Discard())
			assert.Equal(t, tt.wantErr, err)
			assert.Equal(t, tt.wantDefragCalled, d.defraggedCallMap)

		})
	}
}
