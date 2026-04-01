/**
 * Ambient JavaScript API available to Actors.
 *
 * This file contains TypeScript declarations for the globally available
 * functions and objects injected by the Goja runtime in `pkg/actor/js`.
 */

/**
 * Schedules a function to be executed after a specified delay.
 *
 * The underlying Go implementation manages timers in a min-heap to optimize
 * for the closest deadline. Timer callbacks are executed in batches during
 * the runtime's step-based Tick loop to prevent actor starvation.
 *
 * @param callback - The function to execute.
 * @param delayMs - The delay in milliseconds. Defaults to 0 if not provided. Negative values are treated as 0.
 * @param args - Additional arguments to pass to the callback.
 * @returns A numeric identifier for the scheduled timer.
 */
declare function setTimeout(callback: (...args: any[]) => void, delayMs?: number, ...args: any[]): number;

/**
 * Schedules a function to be executed repeatedly, with a fixed time delay between each call.
 *
 * The underlying Go implementation automatically reschedules the timer after each execution
 * if it hasn't been cleared. Processed similarly to setTimeout through the Tick loop.
 *
 * @param callback - The function to execute.
 * @param delayMs - The delay in milliseconds between executions. Defaults to 0 if not provided. Negative values are treated as 0.
 * @param args - Additional arguments to pass to the callback.
 * @returns A numeric identifier for the scheduled interval.
 */
declare function setInterval(callback: (...args: any[]) => void, delayMs?: number, ...args: any[]): number;

/**
 * Cancels a timeout previously established by calling `setTimeout()`.
 *
 * The timer is removed from the internal Go min-heap index if active.
 *
 * @param id - The identifier of the timeout you want to cancel. This ID was returned by the corresponding call to `setTimeout()`.
 */
declare function clearTimeout(id: number): void;

/**
 * Cancels a timed, repeating action which was previously established by a call to `setInterval()`.
 *
 * @param id - The identifier of the repeated action you want to cancel. This ID was returned by the corresponding call to `setInterval()`.
 */
declare function clearInterval(id: number): void;

/**
 * The environment object exposed to the Actor.
 */
declare const env: {
    /**
     * Exposes hardware devices to the Actor.
     *
     * The hardware capabilities (e.g., GPU, Camera) are bound dynamically using `purego`
     * to native libraries, ensuring the project avoids CGO dependencies while retaining access
     * to underlying hardware.
     */
    DEVICES: {
        /**
         * Lists devices by type.
         *
         * @param type - The device type (e.g., 'camera', 'gpu').
         * @returns An array of available device objects.
         */
        list(type: string): any[];

        /**
         * Retrieves a specific device by its identifier.
         *
         * @param id - The unique identifier of the device.
         * @returns The requested device, or null if not found.
         */
        get(id: string): any;
    };
};
