// Package analytics builds bounded presence aggregates and queues background recalculation.
package analytics

import (
	"context"
	domain "github.com/janickiy/meet-space/internal/domain/analytics"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	"github.com/janickiy/meet-space/internal/operations"
	"sort"
	"time"
)

// Repository supplies observed intervals and atomically publishes results only for the current
// job lease.
type Repository interface {
	// Source loads the persisted inputs needed to process conference technical aggregates.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Leased background job and its generation or ownership token.
	//
	// Returns:
	//   - result 1: The timestamp or validity boundary associated with the result.
	//   - result 2: The timestamp or validity boundary associated with the result.
	//   - result 3: The collected interval values.
	//   - result 4: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Source(context.Context, jobs.Job) (time.Time, time.Time, []domain.Interval, error)
	// Save persists analytics results while checking the current job or resource identity.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Leased background job and its generation or ownership token.
	//   - argument 3: Value to validate, normalize or convert.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Save(context.Context, jobs.Job, domain.Conference) error
	// Read reads the analytics state or stream required by the caller.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//
	// Returns:
	//   - result 1: The resulting conference.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Read(context.Context, string, string) (domain.Conference, error)
	// Tick performs one bounded maintenance or delivery iteration after persisted state is
	// available.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Tick(context.Context) error
}

// Service does not depend on speech recognition; transcription failures do not invalidate
// technical conference metrics.
type Service struct {
	Repo    Repository
	Enabled bool
}

// Aggregate computes the bounded technical aggregates of a conference from its persisted source
// data.
//
// Parameters:
//   - conferenceID: Conference identifier limiting the operation scope.
//   - start: Start of the time interval in the units defined by the signature.
//   - end: End of the time interval in the units defined by the signature.
//   - intervals: Ordered participant activity intervals.
//
// Returns:
//   - result 1: The resulting conference.
func Aggregate(conferenceID string, start, end time.Time, intervals []domain.Interval) domain.Conference {
	result := domain.Conference{ConferenceID: conferenceID, Enabled: true, ApproximateSpeaking: true, Timeline: []domain.Point{}, Participants: []domain.Participant{}, DurationMS: max(int64(0), end.Sub(start).Milliseconds())}
	intervalsByParticipant := map[string][]domain.Interval{}
	for _, interval := range intervals {
		if interval.Start.Before(start) {
			interval.Start = start
		}
		if interval.End.After(end) {
			interval.End = end
		}
		if interval.End.After(interval.Start) {
			intervalsByParticipant[interval.ParticipantID] = append(intervalsByParticipant[interval.ParticipantID], interval)
		}
	}
	changes := map[int64]int{}
	participantIDs := make([]string, 0, len(intervalsByParticipant))
	for participantID := range intervalsByParticipant {
		participantIDs = append(participantIDs, participantID)
	}
	sort.Strings(participantIDs)
	for _, participantID := range participantIDs {
		participantIntervals := intervalsByParticipant[participantID]
		sort.Slice(participantIntervals, func(i, j int) bool { return participantIntervals[i].Start.Before(participantIntervals[j].Start) })
		var participationMS int64
		merged := []domain.Interval{}
		for _, interval := range participantIntervals {
			n := len(merged)
			if n == 0 || interval.Start.After(merged[n-1].End) {
				merged = append(merged, interval)
				continue
			}
			if interval.End.After(merged[n-1].End) {
				merged[n-1].End = interval.End
			}
		}
		for _, interval := range merged {
			participationMS += interval.End.Sub(interval.Start).Milliseconds()
			changes[interval.Start.Sub(start).Milliseconds()]++
			changes[interval.End.Sub(start).Milliseconds()]--
		}
		result.Participants = append(result.Participants, domain.Participant{ParticipantID: participantID, ParticipationMS: participationMS})
	}
	result.ParticipantCount = len(result.Participants)
	changeTimes := make([]int64, 0, len(changes))
	for at := range changes {
		changeTimes = append(changeTimes, at)
	}
	sort.Slice(changeTimes, func(i, j int) bool { return changeTimes[i] < changeTimes[j] })
	bucketMS := max(int64(1), (result.DurationMS+598)/599)
	count := 0
	for _, at := range changeTimes {
		count += changes[at]
		point := domain.Point{AtMS: at, Count: max(0, count)}
		n := len(result.Timeline)
		if n > 0 && result.Timeline[n-1].AtMS/bucketMS == at/bucketMS {
			result.Timeline[n-1] = point
		} else {
			result.Timeline = append(result.Timeline, point)
		}
	}
	return result
}

// Handle dispatches the validated analytics operation to its application handler.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - job: Leased background job and its generation or ownership token.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Handle(ctx context.Context, job jobs.Job) error {
	if !s.Enabled {
		return jobs.ErrSkip
	}
	started := time.Now()
	defer func() { operations.Observe("analytics", time.Since(started).Seconds()) }()
	start, end, rows, err := s.Repo.Source(ctx, job)
	if err != nil {
		return err
	}
	return s.Repo.Save(ctx, job, Aggregate(job.ConferenceID, start, end, rows))
}
