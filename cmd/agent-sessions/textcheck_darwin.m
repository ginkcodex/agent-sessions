//go:build darwin && desktop

#import <Foundation/Foundation.h>

// Must match webTextCheckingDefaults in textcheck_darwin.go.
static NSArray<NSString *> *textcheckKeys(void) {
  return @[
    @"WebContinuousSpellCheckingEnabled",
    @"WebGrammarCheckingEnabled",
    @"WebAutomaticSpellingCorrectionEnabled",
    @"WebAutomaticTextReplacementEnabled",
    @"WebAutomaticQuoteSubstitutionEnabled",
    @"WebAutomaticDashSubstitutionEnabled",
    @"WebAutomaticLinkDetectionEnabled",
    @"WebSmartInsertDeleteEnabled",
    @"WebSmartListsEnabled",
  ];
}

void textcheck_register_defaults(void) {
  @autoreleasepool {
    NSMutableDictionary *off = [NSMutableDictionary dictionary];
    for (NSString *key in textcheckKeys()) off[key] = @NO;
    [[NSUserDefaults standardUserDefaults] registerDefaults:off];
  }
}

// 1 or 0 for the effective value, -1 when no domain has the key (WebKit
// would then follow the system setting).
int textcheck_default_value(const char *key) {
  @autoreleasepool {
    id value = [[NSUserDefaults standardUserDefaults] objectForKey:[NSString stringWithUTF8String:key]];
    if (value == nil) return -1;
    return [value boolValue] ? 1 : 0;
  }
}
