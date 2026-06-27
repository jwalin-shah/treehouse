package pool

import (
	"sync"
	"testing"
)

// 1. FuzzPoolAcquireContention targets the state lock and max_trees logic
// during extreme concurrent checkout spikes. We expect no double-allocation
// and no pool size exceedance.
func FuzzPoolAcquireContention(f *testing.F) {
	f.Add(uint(20), uint(5))
	f.Fuzz(func(t *testing.T, agents uint, poolSize uint) {
		if agents == 0 || agents > 50 {
			agents = 20
		}
		if poolSize == 0 || poolSize > 10 {
			poolSize = 5
		}

		repoDir, poolDir := setupRepo(t)
		var wg sync.WaitGroup
		startCh := make(chan struct{})

		paths := make([]string, agents)
		errs := make([]error, agents)

		for i := uint(0); i < agents; i++ {
			wg.Add(1)
			go func(idx uint) {
				defer wg.Done()
				<-startCh
				paths[idx], errs[idx] = Acquire(repoDir, poolDir, int(poolSize), nil)
			}(i)
		}
		close(startCh)
		wg.Wait()

		success := 0
		inUsePaths := make(map[string]bool)
		for i, err := range errs {
			if err == nil {
				success++
				if inUsePaths[paths[i]] {
					t.Fatalf("VULNERABILITY DETECTED: worktree %s acquired by multiple agents simultaneously", paths[i])
				}
				inUsePaths[paths[i]] = true
			}
		}

		if success > int(poolSize) {
			t.Fatalf("VULNERABILITY DETECTED: acquired %d worktrees, exceeding pool size %d", success, poolSize)
		}
	})
}

// 2. FuzzPoolReleaseContention targets state lock when multiple agents
// try to release the same worktree, while others might be acquiring.
func FuzzPoolReleaseContention(f *testing.F) {
	f.Add(uint(20))
	f.Fuzz(func(t *testing.T, agents uint) {
		if agents == 0 || agents > 50 {
			agents = 20
		}

		repoDir, poolDir := setupRepo(t)
		wtPath, err := Acquire(repoDir, poolDir, 5, nil)
		if err != nil {
			t.Fatal(err)
		}

		var wg sync.WaitGroup
		startCh := make(chan struct{})

		for i := uint(0); i < agents; i++ {
			wg.Add(1)
			go func(idx uint) {
				defer wg.Done()
				<-startCh
				if idx%2 == 0 {
					_ = Release(poolDir, wtPath)
				} else {
					_, _ = Acquire(repoDir, poolDir, 5, nil)
				}
			}(i)
		}
		close(startCh)
		wg.Wait()
	})
}

// 3. FuzzPoolDestroyContention targets race conditions between destroying
// a worktree and another agent acquiring it.
func FuzzPoolDestroyContention(f *testing.F) {
	f.Add(uint(15))
	f.Fuzz(func(t *testing.T, agents uint) {
		if agents == 0 || agents > 30 {
			agents = 15
		}

		repoDir, poolDir := setupRepo(t)
		wtPath, err := Acquire(repoDir, poolDir, 5, nil)
		if err != nil {
			t.Fatal(err)
		}
		Release(poolDir, wtPath)

		var wg sync.WaitGroup
		startCh := make(chan struct{})
		
		acquiredBy := make([]string, agents)

		for i := uint(0); i < agents; i++ {
			wg.Add(1)
			go func(idx uint) {
				defer wg.Done()
				<-startCh
				if idx%2 == 0 {
					_ = Destroy(repoDir, poolDir, wtPath, true, nil)
				} else {
					path, err := Acquire(repoDir, poolDir, 5, nil)
					if err == nil {
						acquiredBy[idx] = path
					}
				}
			}(i)
		}
		close(startCh)
		wg.Wait()
	})
}

// 4. FuzzPoolListContention targets the List operation checking state
// and processes while intense Acquires and Releases are mutating the pool.
func FuzzPoolListContention(f *testing.F) {
	f.Add(uint(20))
	f.Fuzz(func(t *testing.T, agents uint) {
		if agents == 0 || agents > 30 {
			agents = 20
		}

		repoDir, poolDir := setupRepo(t)

		var wg sync.WaitGroup
		startCh := make(chan struct{})

		for i := uint(0); i < agents; i++ {
			wg.Add(1)
			go func(idx uint) {
				defer wg.Done()
				<-startCh
				if idx%3 == 0 {
					_, _ = List(poolDir)
				} else if idx%3 == 1 {
					path, err := Acquire(repoDir, poolDir, 2, nil)
					if err == nil {
						_ = Release(poolDir, path)
					}
				} else {
					_, _ = Acquire(repoDir, poolDir, 2, nil)
				}
			}(i)
		}
		close(startCh)
		wg.Wait()
	})
}

// 5. FuzzPoolDestroyAllContention targets bulk destroy against active checkouts.
func FuzzPoolDestroyAllContention(f *testing.F) {
	f.Add(uint(15))
	f.Fuzz(func(t *testing.T, agents uint) {
		if agents == 0 || agents > 30 {
			agents = 15
		}

		repoDir, poolDir := setupRepo(t)

		var wg sync.WaitGroup
		startCh := make(chan struct{})

		for i := uint(0); i < agents; i++ {
			wg.Add(1)
			go func(idx uint) {
				defer wg.Done()
				<-startCh
				if idx%3 == 0 {
					_ = DestroyAll(repoDir, poolDir, true, nil)
				} else if idx%3 == 1 {
					path, err := Acquire(repoDir, poolDir, 5, nil)
					if err == nil {
						Release(poolDir, path)
					}
				} else {
					_, _ = Acquire(repoDir, poolDir, 5, nil)
				}
			}(i)
		}
		close(startCh)
		wg.Wait()
	})
}
