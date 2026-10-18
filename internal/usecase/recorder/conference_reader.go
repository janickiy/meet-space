package recorder

import (
	"context"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/records"
)

// ReadComposite loads an authorized composite-recording card.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The resulting record card.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) ReadComposite(ctx context.Context, id string) (records.RecordCard, error) {
	details, err := s.repository.FindDetailsByUUID(ctx, id)
	if err != nil {
		return records.RecordCard{}, err
	}
	if !records.IsComposite(details.Record) {
		return records.RecordCard{}, apperrors.ErrNotFound
	}
	return s.recordCard(ctx, details)
}

// ReadComposites builds an already-authorized page while retaining the single
// record availability and mode checks. Older repository adapters keep working.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - ids: Resource identifiers to process as a bounded batch.
//
// Returns:
//   - result 1: The collected record card values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) ReadComposites(ctx context.Context, ids []string) ([]records.RecordCard, error) {
	batch, ok := s.repository.(interface {
		// FindDetailsByUUIDs loads details by uui ds from the recording repository.
		//
		// Parameters:
		//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
		//   - argument 2: Resource identifiers to process as a bounded batch.
		//
		// Returns:
		//   - result 1: The collected record details values.
		//   - result 2: nil on success; otherwise the validation, access, transport or persistence
		//     failure.
		FindDetailsByUUIDs(context.Context, []string) ([]records.RecordDetails, error)
	})
	if !ok {
		cards := make([]records.RecordCard, 0, len(ids))
		for _, id := range ids {
			card, err := s.ReadComposite(ctx, id)
			if err != nil {
				return nil, err
			}
			cards = append(cards, card)
		}
		return cards, nil
	}
	rows, err := batch.FindDetailsByUUIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	cards := make([]records.RecordCard, 0, len(rows))
	for _, row := range rows {
		if !records.IsComposite(row.Record) {
			return nil, apperrors.ErrNotFound
		}
		card, err := s.recordCard(ctx, row)
		if err != nil {
			return nil, err
		}
		cards = append(cards, card)
	}
	return cards, nil
}
