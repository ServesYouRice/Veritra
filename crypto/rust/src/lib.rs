//! Crypto integration boundary for Private Messenger.
//!
//! Production MLS/OpenMLS integration is intentionally not implemented in this
//! MVP foundation. Callers must treat `PM_CRYPTO_UNAVAILABLE` as fail-closed.

pub const PM_CRYPTO_UNAVAILABLE: i32 = -1;

#[no_mangle]
pub extern "C" fn pm_crypto_available() -> i32 {
    PM_CRYPTO_UNAVAILABLE
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn production_crypto_fails_closed() {
        assert_eq!(pm_crypto_available(), PM_CRYPTO_UNAVAILABLE);
    }
}
