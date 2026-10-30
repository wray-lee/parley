/// One-time UI flags: "onboarding finished", "Converse explained". They are
/// not secrets; the native build keeps them in the store it already owns and
/// web keeps them in localStorage, because web settings are memory-only.
abstract interface class FirstRunStore {
  Future<bool> seen(String flag);

  Future<void> markSeen(String flag);
}
