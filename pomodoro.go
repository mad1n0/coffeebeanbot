package coffeebeanbot

import (
	"sync"
	"time"
)

// Pomodoro represents a single Pomodoro instance, which can be started and stopped.
// There were a few options I considered when designing this:
// • A sync.Mutex on each Pomodoro, which is locked and unlocked internally to maintain state.
// • Use channels to notify the Pomodoro of events, which handles the state management on its own goroutine
//
// I chose the channel option.  This was to avoid the risks of issues related to locking, as well as
// generally to make it more idiomatic Go.
type Pomodoro struct {
	WorkDuration time.Duration // The duration for a regular Pomodoro work cycle
	OnWorkEnd    func()
	NotifyInfo   NotifyInfo

	cancelChan chan bool // A channel to interrupt our wait if this Pomodoro is cancelled first
	cancel     sync.Once // To ensure we only close the cancelChan once
}

// NotifyInfo contains the necessary information to notify the creating user upon ending the Pomodoro.
type NotifyInfo struct {
	Title   string // The title of the work task
	UserID  string // The UserID to notify
	GuildID string // The Guild (Discord server) that the user created the Pomodoro on
}

// NewPomodoro creates a new Pomodoro and starts it, similar to time.NewTimer. "Start" functionality
// is intentionally omitted to prevent double-starting.
// onWorkEnd is called upon normal Pomodoro ending. NOTE: This does not include cancellation.
func NewPomodoro(workDuration time.Duration, onWorkEnd func(), notify NotifyInfo) *Pomodoro {
	pom := &Pomodoro{
		workDuration,
		onWorkEnd,
		notify,
		make(chan bool),
		sync.Once{},
	}

	go pom.performPom()

	return pom
}

// Cancel is used to cancel a current work cycle. This uses "sync.Once" so we prevent a panic if, for whatever
// reason, the caller is able to call Cancel more than once.
func (pom *Pomodoro) Cancel() {
	pom.cancel.Do(func() {
		close(pom.cancelChan)
	})
}

func (pom *Pomodoro) performPom() {
	workTimer := time.NewTimer(pom.WorkDuration)

	select {
	case <-workTimer.C:
		pom.OnWorkEnd()
	case <-pom.cancelChan:
		workTimer.Stop()
	}
}

// channelPomMap is a map-like structure that has goroutine-safe operations to create Pomodoros on individual channels.
type channelPomMap struct {
	sync.Mutex
	channelToPom map[string]*Pomodoro
}

func newChannelPomMap() channelPomMap {
	return channelPomMap{channelToPom: make(map[string]*Pomodoro)}
}

// CreateIfEmpty will create and start a Pomodoro on the given channel if one does not already exist.
// This method is goroutine-safe.
func (m *channelPomMap) CreateIfEmpty(channel string, duration time.Duration, onWorkEnd func(), notify NotifyInfo) bool {
	m.Lock()
	defer m.Unlock()

	wasCreated := false
	if _, exists := m.channelToPom[channel]; !exists {
		m.channelToPom[channel] = NewPomodoro(duration, onWorkEnd, notify)
		wasCreated = true
	}

	return wasCreated
}

// RemoveIfExists will stop and remove a Pomodoro from the given channel if one exists.
// It returns the NotifyInfo for the channel, if the Pomodoro was removed.
// This method is goroutine-safe.
func (m *channelPomMap) RemoveIfExists(channel string) *NotifyInfo {
	m.Lock()
	defer m.Unlock()

	var removed *NotifyInfo
	if p, exists := m.channelToPom[channel]; exists {
		p.Cancel()
		delete(m.channelToPom, channel)
		removed = &p.NotifyInfo
	}

	return removed
}