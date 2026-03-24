/**
 * Ambient JavaScript API available to Actor instances running in the Goja runtime.
 * This environment is carefully isolated. Functions and properties documented here
 * represent the precise boundary between the JS actor logic and the Go host environment.
 */

/**
 * Schedules a callback to be executed after a specified delay.
 *
 * Note: Timers are managed by a custom `TimerManager` in Go, governed by the Actor's `Tick` cycle.
 * The delay is minimum-bound, but precise execution depends on the runtime's batching limits (e.g. 1000 ops/tick)
 * and the host system's responsiveness.
 *
 * @param callback The function to execute.
 * @param delayMs The minimum delay in milliseconds before the callback is placed in the execution queue.
 * @param args Additional arguments passed through to the callback.
 * @returns A unique numeric timer ID that can be passed to `clearTimeout`.
 */
declare function setTimeout(callback: Function, delayMs?: number, ...args: any[]): number;

/**
 * Cancels a timeout previously established by calling `setTimeout`.
 *
 * @param timeoutId The identifier of the timeout you want to cancel. This ID was returned by the corresponding call to `setTimeout`.
 */
declare function clearTimeout(timeoutId: number): void;

/**
 * Repeatedly calls a function, with a fixed time delay between each call.
 *
 * Note: Execution interval is subject to the same `Tick` lifecycle constraints as `setTimeout`.
 * To prevent actor starvation, intervals are processed within batch limits.
 *
 * @param callback The function to execute.
 * @param delayMs The delay in milliseconds between each execution.
 * @param args Additional arguments passed through to the callback.
 * @returns A unique numeric timer ID that can be passed to `clearInterval`.
 */
declare function setInterval(callback: Function, delayMs?: number, ...args: any[]): number;

/**
 * Cancels a timed, repeating action which was previously established by a call to `setInterval`.
 *
 * @param intervalId The identifier of the repeated action you want to cancel. This ID was returned by the corresponding call to `setInterval`.
 */
declare function clearInterval(intervalId: number): void;

/**
 * Global environment object providing access to hardware and system capabilities.
 */
declare const env: {
    /**
     * Interface for interacting with physical devices (e.g. GPU, Camera) attached to the host.
     *
     * Internally, device interactions are bound to native libraries using `purego` (avoiding CGO).
     * This allows safe, dynamic access and graceful fallback or mocking when hardware libraries are absent.
     */
    DEVICES: {
        /**
         * Retrieves a list of available hardware devices of the specified type.
         *
         * @param type The type of device to query (e.g., 'camera', 'gpu').
         * @returns An array of device identifiers or objects.
         */
        list(type: string): any[];

        /**
         * Acquires a handle to a specific device by its unique identifier.
         *
         * @param id The unique identifier of the device.
         * @returns A device handle exposing type-specific operations, or null if not found/accessible.
         */
        get(id: string): any;
    };
};
