import { invoke } from './api/core.js';

/**
 * Store a value under `key`, encrypted at rest by the platform's secure
 * storage (Android Keystore AES-256-GCM, or the iOS Keychain). Overwrites
 * any existing value for the same key.
 *
 * Pass `{ requireAuth: "os" }` or `{ requireAuth: "password" }` to gate
 * this specific item behind a lock — see {@link AuthMethod}. Use the same
 * `requireAuth` value on {@link getItem} and {@link deleteItem} for this
 * key; it is part of where the item is stored, not just a read-time check.
 */
async function setItem(key, value, options) {
    await invoke("plugin:secure-keystore|set_item", {
        key,
        value,
        requireAuth: options?.requireAuth ?? null,
    });
}
/**
 * Retrieve and decrypt the value stored under `key`, or `null` if nothing
 * is stored for it.
 *
 * If `key` was stored with `requireAuth: "os"`, this triggers the OS
 * biometric/device-credential prompt every call. If it was stored with
 * `requireAuth: "password"`, this throws if the store isn't currently
 * unlocked — call {@link unlockWithPassword} first.
 */
async function getItem(key, options) {
    const result = await invoke("plugin:secure-keystore|get_item", { key, requireAuth: options?.requireAuth ?? null });
    return result.value ?? null;
}
/**
 * Remove the value stored under `key`, if any. Deleting never requires
 * authentication (it doesn't expose the secret), but pass the same
 * `requireAuth` the item was stored with so the plugin deletes it from the
 * right place.
 */
async function deleteItem(key, options) {
    await invoke("plugin:secure-keystore|delete_item", {
        key,
        requireAuth: options?.requireAuth ?? null,
    });
}
/**
 * Sets the app-managed lock password/PIN/pattern used by
 * `requireAuth: "password"` items, on any platform including desktop.
 * Immediately unlocks the session (equivalent to calling
 * {@link unlockWithPassword} right after). Throws if a lock password is
 * already set — use {@link changeLockPassword} to change it.
 */
async function setLockPassword(password) {
    await invoke("plugin:secure-keystore|set_lock_password", { password });
}
/**
 * Changes the lock password. Requires the current password; previously
 * stored `requireAuth: "password"` items stay readable under the new one.
 */
async function changeLockPassword(oldPassword, newPassword) {
    await invoke("plugin:secure-keystore|change_lock_password", {
        oldPassword,
        newPassword,
    });
}
/**
 * Removes the lock password entirely, after verifying it. **Any items
 * previously stored with `requireAuth: "password"` become permanently
 * unreadable** — read and re-save anything you need to keep, without
 * `requireAuth`, before calling this.
 */
async function removeLockPassword(password) {
    await invoke("plugin:secure-keystore|remove_lock_password", { password });
}
/**
 * Verifies `password` and, on success, keeps the derived key in memory for
 * the rest of the app session (or until {@link lock} is called), so
 * subsequent `requireAuth: "password"` calls don't need the password passed
 * again. Call this once, e.g. right after your app's own PIN/password/
 * pattern entry screen. Must be called again after every app restart —
 * nothing is cached to disk.
 */
async function unlockWithPassword(password) {
    await invoke("plugin:secure-keystore|unlock_with_password", { password });
}
/**
 * Drops the in-memory password session. Subsequent `requireAuth:
 * "password"` calls fail until {@link unlockWithPassword} is called again.
 * Use this for an explicit "lock the app" action (e.g. on backgrounding, or
 * a manual lock button) — it does not affect `requireAuth: "os"` items,
 * which are already re-prompted by the OS on every call.
 */
async function lock() {
    await invoke("plugin:secure-keystore|lock_keystore");
}
/** Whether a lock password has been set, and whether it's currently unlocked. */
async function lockStatus() {
    return await invoke("plugin:secure-keystore|lock_status");
}

export { changeLockPassword, deleteItem, getItem, lock, lockStatus, removeLockPassword, setItem, setLockPassword, unlockWithPassword };
