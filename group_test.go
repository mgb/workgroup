package workgroup

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// Allow slice of ints to be in any order by sorting them.
var anyOrder = cmp.Transformer("Sort", func(in []int) []int {
	out := append([]int(nil), in...) // Copy input to avoid mutating it
	sort.Ints(out)
	return out
})

func TestWithContext_normalUseCase(t *testing.T) {
	wg, _ := WithContext[int](context.Background())

	for i := 0; i < 10; i++ {
		i := i
		wg.Go(func(ch chan<- int) error {
			time.Sleep(time.Millisecond)
			ch <- i
			return nil
		})
	}

	results, err := wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(results, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, anyOrder); diff != "" {
		t.Errorf("results mismatch (-want +got):\n%s", diff)
	}

	// Every call to Wait should return the same results.
	results, err = wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(results, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, anyOrder); diff != "" {
		t.Errorf("results mismatch (-want +got):\n%s", diff)
	}
}

func TestWithConcurrency_lotsOfData(t *testing.T) {
	wg, _ := WithConcurrency[int](context.Background(), 2)

	for i := 1; i <= 5; i++ {
		i := i
		wg.Go(func(ch chan<- int) error {
			time.Sleep(time.Millisecond)
			for j := 0; j < i; j++ {
				ch <- j
			}
			return nil
		})
	}

	results, err := wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(results,
		[]int{
			0, 0, 0, 0, 0,
			1, 1, 1, 1,
			2, 2, 2,
			3, 3,
			4,
		},
		anyOrder,
	); diff != "" {
		t.Errorf("results mismatch (-want +got):\n%s", diff)
	}
}

func TestWithConcurrency_error(t *testing.T) {
	wg, _ := WithConcurrency[int](context.Background(), 2)

	// Ensure we only create an error after all other 10 goroutines finish.
	otherCh := make(chan struct{}, 10)
	wg.Go(func(_ chan<- int) error {
		defer close(otherCh)

		for i := 0; i < 10; i++ {
			<-otherCh
		}
		return errors.New("some error")
	})

	// Since we have a concurrency limit greater than 1, that first goroutine will run alongside these.
	for i := 0; i < 10; i++ {
		i := i
		wg.Go(func(ch chan<- int) error {
			time.Sleep(time.Millisecond)
			ch <- i
			otherCh <- struct{}{}
			return nil
		})
	}

	results, err := wg.Wait()
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "some error" {
		t.Errorf("err = %v, want %v", err, "some error")
	}
	if diff := cmp.Diff(results, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, anyOrder); diff != "" {
		t.Errorf("results mismatch (-want +got):\n%s", diff)
	}

	// Every call to Wait should return the same results.
	results, err = wg.Wait()
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "some error" {
		t.Errorf("err = %v, want %v", err, "some error")
	}
	if diff := cmp.Diff(results, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, anyOrder); diff != "" {
		t.Errorf("results mismatch (-want +got):\n%s", diff)
	}
}

func TestWithContext_empty(t *testing.T) {
	wg, _ := WithContext[int](context.Background())

	results, err := wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(results, []int{}, anyOrder); diff != "" {
		t.Errorf("results mismatch (-want +got):\n%s", diff)
	}
}

func TestWithContext_canceledContext_progressMade(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	wg, _ := WithContext[int](ctx)

	for i := 0; i < 10; i++ {
		i := i
		wg.Go(func(ch chan<- int) error {
			time.Sleep(time.Millisecond)
			ch <- i
			return nil
		})
	}

	results, err := wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(results, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, anyOrder); diff != "" {
		t.Errorf("results mismatch (-want +got):\n%s", diff)
	}
}

func TestWithContext_canceledContext_noProgress(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	wg, _ := WithContext[int](ctx)

	for i := 0; i < 10; i++ {
		i := i
		wg.Go(func(ch chan<- int) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Millisecond):
			}

			ch <- i
			return nil
		})
	}

	results, err := wg.Wait()
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want %v", err, context.Canceled)
	}
	if diff := cmp.Diff(results, []int{}, anyOrder); diff != "" {
		t.Errorf("results mismatch (-want +got):\n%s", diff)
	}
}

func TestWithContext_cancelDuringProcessingWithConcurrency(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	wg, _ := WithContext[int](ctx)

	for i := 0; i < 5; i++ {
		i := i
		wg.Go(func(ch chan<- int) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Millisecond):
			}
			ch <- i
			return nil
		})
	}

	cancel()

	for i := 0; i < 5; i++ {
		i := i
		wg.Go(func(ch chan<- int) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Millisecond):
			}
			ch <- i
			return nil
		})
	}

	_, err := wg.Wait()
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want %v", err, context.Canceled)
	}
	// We have no idea what is in results, so we can't assert anything about it.
}

func TestWithSemaphore(t *testing.T) {
	sem := make(chan struct{}, 2)

	wg, _ := WithSemaphore[int](context.Background(), sem)
	wg2, _ := WithSemaphore[int](context.Background(), sem)

	for i := 0; i < 10; i++ {
		i := i
		wg.Go(func(ch chan<- int) error {
			time.Sleep(time.Millisecond)
			ch <- i
			return nil
		})
		wg2.Go(func(ch chan<- int) error {
			time.Sleep(time.Millisecond)
			ch <- i * 2
			return nil
		})
	}

	results, err := wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(results, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, anyOrder); diff != "" {
		t.Errorf("results mismatch (-want +got):\n%s", diff)
	}

	results, err = wg2.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(results, []int{0, 2, 4, 6, 8, 10, 12, 14, 16, 18}, anyOrder); diff != "" {
		t.Errorf("results mismatch (-want +got):\n%s", diff)
	}

	select {
	case sem <- struct{}{}:
	default:
		t.Fatal("semaphore should be open")
	}

	select {
	case sem <- struct{}{}:
	default:
		t.Fatal("semaphore should be open")
	}
}

func TestWithConcurrecny_noConcurrency(t *testing.T) {
	wg, _ := WithConcurrency[int](context.Background(), 0)

	for i := 0; i < 10; i++ {
		i := i
		wg.Go(func(ch chan<- int) error {
			time.Sleep(time.Millisecond)
			ch <- i
			return nil
		})
	}

	results, err := wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(results, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, anyOrder); diff != "" {
		t.Errorf("results mismatch (-want +got):\n%s", diff)
	}
}

func TestWithSemaphore_noConcurrency(t *testing.T) {
	wg, _ := WithSemaphore[int, struct{}](context.Background(), nil)

	for i := 0; i < 10; i++ {
		i := i
		wg.Go(func(ch chan<- int) error {
			time.Sleep(time.Millisecond)
			ch <- i
			return nil
		})
	}

	results, err := wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(results, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, anyOrder); diff != "" {
		t.Errorf("results mismatch (-want +got):\n%s", diff)
	}
}
