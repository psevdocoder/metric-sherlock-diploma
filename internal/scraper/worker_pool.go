package scraper

import (
	"context"
	"sync"

	"git.server.lan/maksim/metric-sherlock-diploma/internal/scrapetask"
)

type workerResult struct {
	taskID int64
	stat   *Statistic
	err    error
}

type WorkerPool struct {
	processor *Processor
	jobs      chan *scrapetask.ScrapeTask
	results   chan *workerResult

	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

func NewWorkerPool(parent context.Context, processor *Processor, workers int) *WorkerPool {
	ctx, cancel := context.WithCancel(parent)

	pool := &WorkerPool{
		processor: processor,
		jobs:      make(chan *scrapetask.ScrapeTask),
		results:   make(chan *workerResult, workers),
		ctx:       ctx,
		cancel:    cancel,
	}

	for i := 0; i < workers; i++ {
		pool.wg.Add(1)
		go pool.worker()
	}

	return pool
}

func (p *WorkerPool) worker() {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return

		case task, ok := <-p.jobs:
			if !ok {
				return
			}

			stat, err := p.processor.Process(p.ctx, task)

			res := &workerResult{
				taskID: task.ID,
				stat:   stat,
				err:    err,
			}

			select {
			case p.results <- res:
			case <-p.ctx.Done():
				return
			}
		}
	}
}

func (p *WorkerPool) Submit(task *scrapetask.ScrapeTask) {
	select {
	case p.jobs <- task:
	case <-p.ctx.Done():
	}
}

func (p *WorkerPool) Results() <-chan *workerResult {
	return p.results
}

func (p *WorkerPool) Stop() {
	p.cancel()

	close(p.jobs)

	p.wg.Wait()

	close(p.results)
}
