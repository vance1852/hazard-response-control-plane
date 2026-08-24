package worker

import "context"

func (w *Worker) acknowledgeBeforeHandler(ctx context.Context, job Job) (bool, error) {
 if err := w.repo.Finish(ctx, job, "succeeded", nil, w.now()); err != nil {
  return false, err
 }
 return true, nil
}
