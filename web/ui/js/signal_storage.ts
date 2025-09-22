// Client-side tab-specific signal persistence using sessionStorage
// This solves the problem of maintaining UI state (like filters) across page navigations within the same tab

interface SignalState {
    connectionId?: string;
    [key: string]: any; // Allow any additional signals
}

export default class SignalStorage {
    private static readonly STORAGE_KEY = 'tickit_signals';

    // Get all signals from sessionStorage
    static getAllSignals(): SignalState {
        const saved = sessionStorage.getItem(this.STORAGE_KEY);
        if (saved) {
            try {
                return JSON.parse(saved) as SignalState;
            } catch (e) {
                console.warn('Failed to parse saved signals:', e);
                return {};
            }
        }
        return {};
    }

    // Save all signals to sessionStorage
    static saveAllSignals(signals: SignalState): void {
        sessionStorage.setItem(this.STORAGE_KEY, JSON.stringify(signals));
    }

    // Set signals from a patch object (merges with existing signals)
    static setSignal(patch: Partial<SignalState>): void {
        const signals = this.getAllSignals();
        Object.assign(signals, patch);
        this.saveAllSignals(signals);
        console.log('Saved signal patch:', patch);
    }

    // Load initial state with defaults, optionally setting connectionId from backend
    static loadInitialState(connectionId: string): SignalState {
        this.setSignal({ connectionId }); // Ensure connectionId is always set
        return {
            connectionId,
            ...this.getAllSignals()
        };
    }
}

// Example Datastar integration:
/*
<!-- Initialize with saved state and auto-save on changes -->
<div data-signals='SignalStorage.loadInitialState()' 
     data-on-signal-patch="SignalStorage.setSignal(patch)">

<!-- Any UI elements that update signals -->
<select data-model="filter.project">...</select>
<button data-on-click="$signals.sort = {by: 'created', dir: 'asc'}">Sort</button>

</div>
*/