package users

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// PerformanceTestSuite tests performance characteristics of the user management system
type PerformanceTestSuite struct {
	suite.Suite
	ctx        context.Context
	store      UserStore
	tempDBFile string
}

func (suite *PerformanceTestSuite) SetupSuite() {
	suite.ctx = context.Background()
	
	// Create temporary database
	suite.tempDBFile = fmt.Sprintf("test_performance_%d.db", time.Now().UnixNano())
	
	var err error
	suite.store, err = NewUserStore(suite.ctx, suite.tempDBFile)
	suite.Require().NoError(err)
}

func (suite *PerformanceTestSuite) TearDownSuite() {
	if suite.store != nil {
		// Close the store if it has a Close method
		if closer, ok := suite.store.(interface{ Close() error }); ok {
			closer.Close()
		}
	}
	if suite.tempDBFile != "" {
		os.Remove(suite.tempDBFile)
	}
}

// BenchmarkUserCreation benchmarks user creation performance
func (suite *PerformanceTestSuite) TestUserCreationPerformance() {
	suite.Run("Sequential User Creation", func() {
		const numUsers = 1000
		users := make([]*User, numUsers)
		
		// Prepare user data
		for i := 0; i < numUsers; i++ {
			users[i] = &User{
				ID:            uuid.New().String(),
				Email:         fmt.Sprintf("perf%d@example.com", i),
				Username:      fmt.Sprintf("perf%d", i),
				FirstName:     "Performance",
				LastName:      "User",
				Role:          "Viewer",
				Status:        "Active",
				EmailVerified: true,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			}
		}
		
		// Measure creation time
		start := time.Now()
		
		for _, user := range users {
			err := suite.store.CreateUser(suite.ctx, user)
			suite.NoError(err)
		}
		
		duration := time.Since(start)
		avgTime := duration / time.Duration(numUsers)
		
		suite.T().Logf("Created %d users in %v (avg: %v per user)", numUsers, duration, avgTime)
		suite.T().Logf("Throughput: %.2f users/second", float64(numUsers)/duration.Seconds())
		
		// Performance assertions
		suite.Less(avgTime, 10*time.Millisecond, "Average user creation should be under 10ms")
		suite.Less(duration, 10*time.Second, "Total creation time should be reasonable")
	})
	
	suite.Run("Concurrent User Creation", func() {
		const numUsers = 1000
		const numWorkers = 10
		
		userChan := make(chan *User, numUsers)
		errorChan := make(chan error, numUsers)
		
		// Prepare user data
		for i := 0; i < numUsers; i++ {
			user := &User{
				ID:            uuid.New().String(),
				Email:         fmt.Sprintf("concurrent%d@example.com", i),
				Username:      fmt.Sprintf("concurrent%d", i),
				FirstName:     "Concurrent",
				LastName:      "User",
				Role:          "Viewer",
				Status:        "Active",
				EmailVerified: true,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			}
			userChan <- user
		}
		close(userChan)
		
		// Start workers
		var wg sync.WaitGroup
		start := time.Now()
		
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for user := range userChan {
					err := suite.store.CreateUser(suite.ctx, user)
					errorChan <- err
				}
			}()
		}
		
		wg.Wait()
		close(errorChan)
		
		duration := time.Since(start)
		
		// Check for errors
		errorCount := 0
		for err := range errorChan {
			if err != nil {
				errorCount++
			}
		}
		
		suite.Equal(0, errorCount, "No errors should occur during concurrent creation")
		
		avgTime := duration / time.Duration(numUsers)
		suite.T().Logf("Created %d users concurrently in %v (avg: %v per user)", numUsers, duration, avgTime)
		suite.T().Logf("Throughput: %.2f users/second", float64(numUsers)/duration.Seconds())
		
		// Concurrent creation should be faster than sequential
		suite.Less(duration, 5*time.Second, "Concurrent creation should be efficient")
	})
}

// TestUserRetrievalPerformance tests user lookup performance
func (suite *PerformanceTestSuite) TestUserRetrievalPerformance() {
	// Create test users first
	const numUsers = 10000
	userIDs := make([]string, numUsers)
	emails := make([]string, numUsers)
	
	suite.T().Log("Setting up test data...")
	for i := 0; i < numUsers; i++ {
		userID := uuid.New().String()
		email := fmt.Sprintf("lookup%d@example.com", i)
		
		user := &User{
			ID:            userID,
			Email:         email,
			Username:      fmt.Sprintf("lookup%d", i),
			FirstName:     "Lookup",
			LastName:      "User",
			Role:          "Viewer",
			Status:        "Active",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		
		err := suite.store.CreateUser(suite.ctx, user)
		suite.Require().NoError(err)
		
		userIDs[i] = userID
		emails[i] = email
	}
	
	suite.Run("User Lookup by ID", func() {
		const numLookups = 1000
		
		start := time.Now()
		
		for i := 0; i < numLookups; i++ {
			userID := userIDs[i%len(userIDs)]
			user, err := suite.store.GetUser(suite.ctx, userID)
			suite.NoError(err)
			suite.NotNil(user)
		}
		
		duration := time.Since(start)
		avgTime := duration / time.Duration(numLookups)
		
		suite.T().Logf("Performed %d ID lookups in %v (avg: %v per lookup)", numLookups, duration, avgTime)
		suite.T().Logf("Throughput: %.2f lookups/second", float64(numLookups)/duration.Seconds())
		
		// ID lookups should be very fast
		suite.Less(avgTime, 1*time.Millisecond, "ID lookup should be under 1ms")
	})
	
	suite.Run("User Lookup by Email", func() {
		const numLookups = 1000
		
		start := time.Now()
		
		for i := 0; i < numLookups; i++ {
			email := emails[i%len(emails)]
			user, err := suite.store.GetUserByEmail(suite.ctx, email)
			suite.NoError(err)
			suite.NotNil(user)
		}
		
		duration := time.Since(start)
		avgTime := duration / time.Duration(numLookups)
		
		suite.T().Logf("Performed %d email lookups in %v (avg: %v per lookup)", numLookups, duration, avgTime)
		suite.T().Logf("Throughput: %.2f lookups/second", float64(numLookups)/duration.Seconds())
		
		// Email lookups should be reasonably fast (depends on indexing)
		suite.Less(avgTime, 5*time.Millisecond, "Email lookup should be under 5ms")
	})
	
	suite.Run("Concurrent User Lookups", func() {
		const numLookups = 1000
		const numWorkers = 20
		
		lookupChan := make(chan string, numLookups)
		resultChan := make(chan time.Duration, numLookups)
		
		// Prepare lookup data
		for i := 0; i < numLookups; i++ {
			lookupChan <- userIDs[i%len(userIDs)]
		}
		close(lookupChan)
		
		// Start workers
		var wg sync.WaitGroup
		overallStart := time.Now()
		
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for userID := range lookupChan {
					start := time.Now()
					user, err := suite.store.GetUser(suite.ctx, userID)
					duration := time.Since(start)
					
					suite.NoError(err)
					suite.NotNil(user)
					resultChan <- duration
				}
			}()
		}
		
		wg.Wait()
		close(resultChan)
		
		overallDuration := time.Since(overallStart)
		
		// Analyze results
		var totalTime time.Duration
		maxTime := time.Duration(0)
		minTime := time.Duration(1 * time.Hour) // Large initial value
		
		for duration := range resultChan {
			totalTime += duration
			if duration > maxTime {
				maxTime = duration
			}
			if duration < minTime {
				minTime = duration
			}
		}
		
		avgTime := totalTime / time.Duration(numLookups)
		
		suite.T().Logf("Concurrent lookups: %d lookups in %v", numLookups, overallDuration)
		suite.T().Logf("Average lookup time: %v", avgTime)
		suite.T().Logf("Min lookup time: %v", minTime)
		suite.T().Logf("Max lookup time: %v", maxTime)
		suite.T().Logf("Throughput: %.2f lookups/second", float64(numLookups)/overallDuration.Seconds())
		
		// Concurrent lookups should maintain good performance
		suite.Less(avgTime, 10*time.Millisecond, "Average concurrent lookup should be under 10ms")
		suite.Less(maxTime, 100*time.Millisecond, "Max lookup time should be reasonable")
	})
}

// TestPasswordHashingPerformance tests password hashing performance
func (suite *PerformanceTestSuite) TestPasswordHashingPerformance() {
	passwordHasher := NewDefaultBcryptPasswordHasher()
	password := "TestPassword123!"
	
	suite.Run("Password Hashing", func() {
		const numHashes = 100
		
		start := time.Now()
		
		for i := 0; i < numHashes; i++ {
			_, err := passwordHasher.HashPassword(password)
			suite.NoError(err)
		}
		
		duration := time.Since(start)
		avgTime := duration / time.Duration(numHashes)
		
		suite.T().Logf("Hashed %d passwords in %v (avg: %v per hash)", numHashes, duration, avgTime)
		suite.T().Logf("Throughput: %.2f hashes/second", float64(numHashes)/duration.Seconds())
		
		// Password hashing should be intentionally slow for security
		suite.Greater(avgTime, 50*time.Millisecond, "Password hashing should be slow enough for security")
		suite.Less(avgTime, 500*time.Millisecond, "Password hashing should not be too slow for usability")
	})
	
	suite.Run("Password Verification", func() {
		// Hash password once
		hashedPassword, err := passwordHasher.HashPassword(password)
		suite.Require().NoError(err)
		
		const numVerifications = 100
		
		start := time.Now()
		
		for i := 0; i < numVerifications; i++ {
			err := passwordHasher.CheckPassword(hashedPassword, password)
			suite.NoError(err)
		}
		
		duration := time.Since(start)
		avgTime := duration / time.Duration(numVerifications)
		
		suite.T().Logf("Verified %d passwords in %v (avg: %v per verification)", numVerifications, duration, avgTime)
		suite.T().Logf("Throughput: %.2f verifications/second", float64(numVerifications)/duration.Seconds())
		
		// Password verification should have similar timing to hashing
		suite.Greater(avgTime, 50*time.Millisecond, "Password verification should be slow enough for security")
		suite.Less(avgTime, 500*time.Millisecond, "Password verification should not be too slow for usability")
	})
	
	suite.Run("Concurrent Password Operations", func() {
		const numOperations = 50
		const numWorkers = 10
		
		operationChan := make(chan string, numOperations)
		resultChan := make(chan time.Duration, numOperations)
		
		// Prepare operations
		for i := 0; i < numOperations; i++ {
			operationChan <- fmt.Sprintf("password%d", i)
		}
		close(operationChan)
		
		// Start workers
		var wg sync.WaitGroup
		overallStart := time.Now()
		
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for pwd := range operationChan {
					start := time.Now()
					_, err := passwordHasher.HashPassword(pwd)
					duration := time.Since(start)
					
					suite.NoError(err)
					resultChan <- duration
				}
			}()
		}
		
		wg.Wait()
		close(resultChan)
		
		overallDuration := time.Since(overallStart)
		
		// Analyze results
		var totalTime time.Duration
		for duration := range resultChan {
			totalTime += duration
		}
		
		avgTime := totalTime / time.Duration(numOperations)
		
		suite.T().Logf("Concurrent password hashing: %d operations in %v", numOperations, overallDuration)
		suite.T().Logf("Average operation time: %v", avgTime)
		suite.T().Logf("Throughput: %.2f operations/second", float64(numOperations)/overallDuration.Seconds())
		
		// Concurrent operations should scale reasonably
		suite.Less(overallDuration, 30*time.Second, "Concurrent password operations should complete in reasonable time")
	})
}

// TestSessionManagementPerformance tests session-related performance
func (suite *PerformanceTestSuite) TestSessionManagementPerformance() {
	// Create test users first
	const numUsers = 1000
	userIDs := make([]string, numUsers)
	
	for i := 0; i < numUsers; i++ {
		userID := uuid.New().String()
		user := &User{
			ID:            userID,
			Email:         fmt.Sprintf("session%d@example.com", i),
			Username:      fmt.Sprintf("session%d", i),
			FirstName:     "Session",
			LastName:      "User",
			Role:          "Viewer",
			Status:        "Active",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		
		err := suite.store.CreateUser(suite.ctx, user)
		suite.Require().NoError(err)
		userIDs[i] = userID
	}
	
	suite.Run("Session Creation", func() {
		const numSessions = 5000
		
		start := time.Now()
		
		for i := 0; i < numSessions; i++ {
			session := &UserSession{
				ID:        uuid.New().String(),
				UserID:    userIDs[i%len(userIDs)],
				SessionID: fmt.Sprintf("session_%d_%s", i, uuid.New().String()),
				ExpiresAt: time.Now().Add(time.Hour),
				CreatedAt: time.Now(),
				IPAddress: fmt.Sprintf("192.168.1.%d", (i%254)+1),
				UserAgent: "performance-test-agent",
			}
			
			err := suite.store.CreateUserSession(suite.ctx, session)
			suite.NoError(err)
		}
		
		duration := time.Since(start)
		avgTime := duration / time.Duration(numSessions)
		
		suite.T().Logf("Created %d sessions in %v (avg: %v per session)", numSessions, duration, avgTime)
		suite.T().Logf("Throughput: %.2f sessions/second", float64(numSessions)/duration.Seconds())
		
		// Session creation should be fast
		suite.Less(avgTime, 5*time.Millisecond, "Session creation should be under 5ms")
	})
	
	suite.Run("Session Cleanup", func() {
		// Create expired sessions
		const numExpiredSessions = 1000
		
		for i := 0; i < numExpiredSessions; i++ {
			session := &UserSession{
				ID:        uuid.New().String(),
				UserID:    userIDs[i%len(userIDs)],
				SessionID: fmt.Sprintf("expired_%d_%s", i, uuid.New().String()),
				ExpiresAt: time.Now().Add(-time.Hour), // Expired
				CreatedAt: time.Now().Add(-2 * time.Hour),
				IPAddress: fmt.Sprintf("192.168.2.%d", (i%254)+1),
				UserAgent: "expired-test-agent",
			}
			
			err := suite.store.CreateUserSession(suite.ctx, session)
			suite.Require().NoError(err)
		}
		
		// Measure cleanup time
		start := time.Now()
		deletedCount, err := suite.store.DeleteExpiredSessions(suite.ctx)
		duration := time.Since(start)
		
		suite.NoError(err)
		suite.GreaterOrEqual(deletedCount, numExpiredSessions)
		
		suite.T().Logf("Cleaned up %d expired sessions in %v", deletedCount, duration)
		suite.T().Logf("Cleanup rate: %.2f sessions/second", float64(deletedCount)/duration.Seconds())
		
		// Cleanup should be efficient
		suite.Less(duration, 5*time.Second, "Session cleanup should be efficient")
	})
}

// TestMemoryUsage tests memory usage characteristics
func (suite *PerformanceTestSuite) TestMemoryUsage() {
	suite.Run("Memory Usage During Bulk Operations", func() {
		// Force garbage collection to get baseline
		runtime.GC()
		runtime.GC() // Call twice to ensure cleanup
		
		var m1 runtime.MemStats
		runtime.ReadMemStats(&m1)
		
		// Perform bulk operations
		const numUsers = 10000
		users := make([]*User, numUsers)
		
		for i := 0; i < numUsers; i++ {
			users[i] = &User{
				ID:            uuid.New().String(),
				Email:         fmt.Sprintf("memory%d@example.com", i),
				Username:      fmt.Sprintf("memory%d", i),
				FirstName:     "Memory",
				LastName:      "User",
				Role:          "Viewer",
				Status:        "Active",
				EmailVerified: true,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			}
			
			err := suite.store.CreateUser(suite.ctx, users[i])
			suite.NoError(err)
		}
		
		// Force garbage collection and measure
		runtime.GC()
		runtime.GC()
		
		var m2 runtime.MemStats
		runtime.ReadMemStats(&m2)
		
		memoryUsed := m2.Alloc - m1.Alloc
		memoryPerUser := memoryUsed / uint64(numUsers)
		
		suite.T().Logf("Memory used for %d users: %d bytes", numUsers, memoryUsed)
		suite.T().Logf("Memory per user: %d bytes", memoryPerUser)
		suite.T().Logf("Total allocations: %d", m2.TotalAlloc-m1.TotalAlloc)
		suite.T().Logf("GC cycles: %d", m2.NumGC-m1.NumGC)
		
		// Memory usage should be reasonable
		suite.Less(memoryPerUser, uint64(1024), "Memory per user should be under 1KB")
		
		// Clear references to help GC
		users = nil
		runtime.GC()
	})
}

// TestDatabasePerformance tests database-specific performance characteristics
func (suite *PerformanceTestSuite) TestDatabasePerformance() {
	suite.Run("Transaction Performance", func() {
		const numTransactions = 1000
		
		start := time.Now()
		
		for i := 0; i < numTransactions; i++ {
			// Each user creation is typically a transaction
			user := &User{
				ID:            uuid.New().String(),
				Email:         fmt.Sprintf("tx%d@example.com", i),
				Username:      fmt.Sprintf("tx%d", i),
				FirstName:     "Transaction",
				LastName:      "User",
				Role:          "Viewer",
				Status:        "Active",
				EmailVerified: true,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			}
			
			err := suite.store.CreateUser(suite.ctx, user)
			suite.NoError(err)
		}
		
		duration := time.Since(start)
		avgTime := duration / time.Duration(numTransactions)
		
		suite.T().Logf("Completed %d transactions in %v (avg: %v per transaction)", numTransactions, duration, avgTime)
		suite.T().Logf("Throughput: %.2f transactions/second", float64(numTransactions)/duration.Seconds())
		
		// Transaction performance should be reasonable
		suite.Less(avgTime, 10*time.Millisecond, "Average transaction time should be under 10ms")
	})
	
	suite.Run("Query Performance with Large Dataset", func() {
		// Create a large dataset first
		const datasetSize = 50000
		
		suite.T().Logf("Creating dataset of %d users...", datasetSize)
		setupStart := time.Now()
		
		for i := 0; i < datasetSize; i++ {
			user := &User{
				ID:            uuid.New().String(),
				Email:         fmt.Sprintf("large%d@example.com", i),
				Username:      fmt.Sprintf("large%d", i),
				FirstName:     "Large",
				LastName:      "Dataset",
				Role:          "Viewer",
				Status:        "Active",
				EmailVerified: true,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			}
			
			err := suite.store.CreateUser(suite.ctx, user)
			suite.Require().NoError(err)
		}
		
		setupDuration := time.Since(setupStart)
		suite.T().Logf("Dataset creation took %v", setupDuration)
		
		// Test query performance
		const numQueries = 1000
		
		start := time.Now()
		
		for i := 0; i < numQueries; i++ {
			email := fmt.Sprintf("large%d@example.com", i%datasetSize)
			user, err := suite.store.GetUserByEmail(suite.ctx, email)
			suite.NoError(err)
			suite.NotNil(user)
		}
		
		duration := time.Since(start)
		avgTime := duration / time.Duration(numQueries)
		
		suite.T().Logf("Performed %d queries on dataset of %d users in %v (avg: %v per query)", 
			numQueries, datasetSize, duration, avgTime)
		suite.T().Logf("Throughput: %.2f queries/second", float64(numQueries)/duration.Seconds())
		
		// Query performance should remain good even with large dataset
		suite.Less(avgTime, 10*time.Millisecond, "Query performance should remain good with large dataset")
	})
}

// Run the performance test suite
func TestPerformanceTestSuite(t *testing.T) {
	suite.Run(t, new(PerformanceTestSuite))
}

// Benchmark functions for go test -bench

func BenchmarkUserCreation(b *testing.B) {
	ctx := context.Background()
	store, err := NewUserStore(ctx, ":memory:")
	require.NoError(b, err)
	defer store.Close()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		user := &User{
			ID:            uuid.New().String(),
			Email:         fmt.Sprintf("bench%d@example.com", i),
			Username:      fmt.Sprintf("bench%d", i),
			FirstName:     "Benchmark",
			LastName:      "User",
			Role:          "Viewer",
			Status:        "Active",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		
		err := store.CreateUser(ctx, user)
		require.NoError(b, err)
	}
}

func BenchmarkUserLookupByID(b *testing.B) {
	ctx := context.Background()
	store, err := NewUserStore(ctx, ":memory:")
	require.NoError(b, err)
	defer store.Close()
	
	// Create test user
	userID := uuid.New().String()
	user := &User{
		ID:            userID,
		Email:         "benchmark@example.com",
		Username:      "benchmarkuser",
		FirstName:     "Benchmark",
		LastName:      "User",
		Role:          "Viewer",
		Status:        "Active",
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	
	err = store.CreateUser(ctx, user)
	require.NoError(b, err)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_, err := store.GetUser(ctx, userID)
		require.NoError(b, err)
	}
}

func BenchmarkUserLookupByEmail(b *testing.B) {
	ctx := context.Background()
	store, err := NewUserStore(ctx, ":memory:")
	require.NoError(b, err)
	defer store.Close()
	
	// Create test user
	email := "benchmark@example.com"
	user := &User{
		ID:            uuid.New().String(),
		Email:         email,
		Username:      "benchmarkuser",
		FirstName:     "Benchmark",
		LastName:      "User",
		Role:          "Viewer",
		Status:        "Active",
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	
	err = store.CreateUser(ctx, user)
	require.NoError(b, err)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_, err := store.GetUserByEmail(ctx, email)
		require.NoError(b, err)
	}
}

func BenchmarkPasswordHashing(b *testing.B) {
	passwordHasher := NewDefaultBcryptPasswordHasher()
	password := "BenchmarkPassword123!"
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_, err := passwordHasher.HashPassword(password)
		require.NoError(b, err)
	}
}

func BenchmarkPasswordVerification(b *testing.B) {
	passwordHasher := NewDefaultBcryptPasswordHasher()
	password := "BenchmarkPassword123!"
	
	// Hash password once
	hashedPassword, err := passwordHasher.HashPassword(password)
	require.NoError(b, err)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		err := passwordHasher.CheckPassword(hashedPassword, password)
		require.NoError(b, err)
	}
}