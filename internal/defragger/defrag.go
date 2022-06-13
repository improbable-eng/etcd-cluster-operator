package defragger

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/improbable-eng/etcd-cluster-operator/internal/etcd"
)

type usedSpaceChecker interface {
	UsedSpacePercentage(context.Context, etcd.Member) (int, error)
}

type defragger interface {
	Defragment(context.Context, etcd.Member) error
}

// DefragIfNecessary will defrag each etcd member if necessary, it will return the last error encountered
func DefragIfNecessary(ctx context.Context, defragThreshold uint, memberList []etcd.Member, s usedSpaceChecker, d defragger, log logr.Logger) error {
	var returnErr error
	for _, member := range memberList {
		log = log.WithValues("member_id", member.ID, "defrag_threshold", defragThreshold)

		usedSpace, err := s.UsedSpacePercentage(ctx, member)
		if err != nil {
			log.Error(err, "unable to check if defrag is necessary")
			returnErr = err
			continue
		}

		log = log.WithValues("percentage_used_space", usedSpace)

		if usedSpace <= int(defragThreshold) {
			log.Info("defrag not necessary")
			continue
		}

		log.Info("defrag necessary - defragging")

		err = DefragMember(ctx, member, d, log)
		if err != nil {
			returnErr = err
			continue
		}
	}

	return returnErr
}

// Defrag will defrag each etcd member, it will return the last error encountered
func Defrag(ctx context.Context, memberList []etcd.Member, d defragger, log logr.Logger) error {
	var returnErr error

	for _, member := range memberList {
		err := DefragMember(ctx, member, d, log)
		if err != nil {
			returnErr = err
			continue
		}
	}

	return returnErr
}

// DefragMember will degfrag a member.
func DefragMember(ctx context.Context, member etcd.Member, d defragger, log logr.Logger) error {
	err := d.Defragment(ctx, member)
	if err != nil {
		log.Error(err, "unable to defrag etcd")
		return err
	}

	log.Info("defrag complete")

	return nil
}
