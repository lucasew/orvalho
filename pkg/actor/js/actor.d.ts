/**
 * Defines the ambient JavaScript environment provided to Actors running in the Orvalho mesh runtime.
 * This environment is executed within an isolated `goja` VM and relies on a step-based `Tick` loop
 * managed by Go.
 */

/**
 * Global interface defining the hardware and system capabilities exposed to the Actor.
 */
interface Environment {
    /**
     * Exposes dynamic hardware bindings (e.g., GPU, Camera).
     * Underlying implementation uses `purego` to bind native libraries securely
     * without CGO dependencies.
     */
    DEVICES: {
        /**
         * Lists all available devices of a specific type.
         * @param type - The string identifier for the device type (e.g., 'camera', 'gpu').
         * @returns An array of device identifier strings or configuration objects.
         */
        list(type: string): any[];

        /**
         * Retrieves a specific device instance by its identifier.
         * @param id - The unique identifier of the device.
         * @returns An interface to interact with the requested device.
         */
        get(id: string): any;
    };
}

declare const env: Environment;

/**
 * Schedules a callback to be executed after a minimum delay.
 * Timers are managed by a Go heap and are processed during the `Tick` loop, meaning
 * execution is dependent on the host application pumping the Actor state.
 *
 * @param callback - The function to execute.
 * @param delayMs - The delay in milliseconds. Note that actual execution may be delayed
 *                  further if the `Tick` loop is blocked or busy.
 * @param args - Additional arguments to pass to the callback.
 * @returns A numeric identifier for the scheduled timer.
 */
declare function setTimeout(
    callback: (...args: any[]) => void,
    delayMs?: number,
    ...args: any[]
): number;

/**
 * Clears a pending timeout scheduled via `setTimeout`.
 * If the provided identifier does not exist or has already executed, this is a no-op.
 *
 * @param timeoutId - The numeric identifier returned by `setTimeout`.
 */
declare function clearTimeout(timeoutId: number): void;

/**
 * Schedules a callback to be executed repeatedly at a specified interval.
 * Intervals are managed on the Go side and rescheduled dynamically after each execution.
 *
 * @param callback - The function to execute.
 * @param intervalMs - The interval in milliseconds between executions.
 * @param args - Additional arguments to pass to the callback.
 * @returns A numeric identifier for the scheduled interval.
 */
declare function setInterval(
    callback: (...args: any[]) => void,
    intervalMs?: number,
    ...args: any[]
): number;

/**
 * Clears an active interval scheduled via `setInterval`.
 *
 * @param intervalId - The numeric identifier returned by `setInterval`.
 */
declare function clearInterval(intervalId: number): void;
