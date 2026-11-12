#import <Foundation/Foundation.h>
#include <stdlib.h>
#include <string.h>

@interface QZAuthSessionDelegate : NSObject <NSURLSessionTaskDelegate>
@end
@implementation QZAuthSessionDelegate
- (void)URLSession:(NSURLSession *)session task:(NSURLSessionTask *)task
 willPerformHTTPRedirection:(NSHTTPURLResponse *)response newRequest:(NSURLRequest *)request
 completionHandler:(void (^)(NSURLRequest *))completionHandler {
    // Go validates the destination before replaying the original signed POST.
    completionHandler(nil);
}
@end

// Use delegate delivery for restore-image ranges: cancel at the headers if
// a server ignores Range, and never accumulate more than one small chunk.
@interface QZResourceSessionDelegate : QZAuthSessionDelegate <NSURLSessionDataDelegate>
@property(nonatomic) NSUInteger limit;
@property(nonatomic) BOOL requiresRange;
@property(nonatomic, strong) NSMutableData *data;
@property(nonatomic, copy) void (^completion)(NSData *, NSURLResponse *, NSError *);
@end
@implementation QZResourceSessionDelegate
- (void)URLSession:(NSURLSession *)session dataTask:(NSURLSessionDataTask *)task
 didReceiveResponse:(NSURLResponse *)response completionHandler:(void (^)(NSURLSessionResponseDisposition))handler {
    if (![response isKindOfClass:NSHTTPURLResponse.class]) {
        handler(NSURLSessionResponseCancel);
        return;
    }
    NSInteger status = ((NSHTTPURLResponse *)response).statusCode;
    BOOL ignoredRange = self.requiresRange && status >= 200 && status < 300 && status != 206;
    handler(ignoredRange || response.expectedContentLength > (long long)self.limit
        ? NSURLSessionResponseCancel : NSURLSessionResponseAllow);
}
- (void)URLSession:(NSURLSession *)session dataTask:(NSURLSessionDataTask *)task didReceiveData:(NSData *)data {
    if (data.length > self.limit - self.data.length) {
        [task cancel];
        return;
    }
    [self.data appendData:data];
}
- (void)URLSession:(NSURLSession *)session task:(NSURLSessionTask *)task didCompleteWithError:(NSError *)error {
    self.completion(self.data, task.response, error);
}
@end

// Called only on Go's background worker. No credentials, URLs or response
// bodies are logged or persisted. The caller owns the returned malloc buffer.
char *QZNativeHTTP(const char *requestJSON) {
    @autoreleasepool {
        NSData *input = requestJSON ? [NSData dataWithBytes:requestJSON length:strlen(requestJSON)] : nil;
        NSDictionary *values = input ? [NSJSONSerialization JSONObjectWithData:input options:0 error:nil] : nil;
        NSURL *url = [values isKindOfClass:NSDictionary.class] ? [NSURL URLWithString:values[@"url"]] : nil;
        if (!url || ![@[@"https", @"http"] containsObject:url.scheme.lowercaseString]) return strdup("{\"errorCode\":-1000}");
        NSTimeInterval timeout = MIN(30, MAX(0.1, [values[@"timeout"] doubleValue]));
        NSMutableURLRequest *request = [NSMutableURLRequest requestWithURL:url cachePolicy:NSURLRequestReloadIgnoringLocalCacheData timeoutInterval:timeout];
        request.HTTPMethod = values[@"method"];
        NSData *body = [[NSData alloc] initWithBase64EncodedString:[values[@"body"] isKindOfClass:NSString.class] ? values[@"body"] : @"" options:0];
        if (body.length > 0) request.HTTPBody = body;
        request.HTTPShouldHandleCookies = NO;
        [values[@"headers"] enumerateKeysAndObjectsUsingBlock:^(NSString *key, NSArray *items, BOOL *stop) {
            if ([key caseInsensitiveCompare:@"Content-Length"] != NSOrderedSame) {
                [request setValue:[items componentsJoinedByString:@"; "] forHTTPHeaderField:key];
            }
        }];
        NSURLSessionConfiguration *config = NSURLSessionConfiguration.ephemeralSessionConfiguration;
        config.HTTPCookieStorage = nil; config.HTTPShouldSetCookies = NO;
        config.URLCache = nil; config.URLCredentialStorage = nil;
        config.timeoutIntervalForRequest = timeout; config.timeoutIntervalForResource = timeout;
        BOOL publicResource = [values[@"publicResource"] boolValue];
        dispatch_semaphore_t done = dispatch_semaphore_create(0);
        NSLock *lock = [NSLock new];
        __block NSDictionary *result;
        void (^completion)(NSData *, NSURLResponse *, NSError *) = ^(NSData *data, NSURLResponse *response, NSError *error) {
            NSDictionary *completed;
            if (error || ![response isKindOfClass:NSHTTPURLResponse.class]) {
                completed = @{@"errorCode": @(error ? error.code : -1011)};
            } else if (data.length > 2 * 1024 * 1024) {
                completed = @{@"errorCode": @(-1103)};
            } else {
                NSHTTPURLResponse *http = (NSHTTPURLResponse *)response;
                NSMutableDictionary *headers = [NSMutableDictionary new];
                [http.allHeaderFields enumerateKeysAndObjectsUsingBlock:^(id key, id value, BOOL *stop) {
                    headers[[key description]] = @[[value description]];
                }];
                NSMutableArray *cookies = [NSMutableArray new];
                for (NSHTTPCookie *cookie in [NSHTTPCookie cookiesWithResponseHeaderFields:http.allHeaderFields forURL:http.URL]) {
                    [cookies addObject:@{@"Name":cookie.name, @"Value":cookie.value, @"Domain":cookie.domain, @"Path":cookie.path,
                        @"Expires":@(cookie.expiresDate ? cookie.expiresDate.timeIntervalSince1970 : 0),
                        @"Secure":@(cookie.secure), @"HTTPOnly":@(cookie.HTTPOnly)}];
                }
                completed = @{@"status":@(http.statusCode), @"headers":headers, @"body":[(data ?: [NSData data]) base64EncodedStringWithOptions:0], @"cookies":cookies};
            }
            [lock lock]; result = completed; [lock unlock];
            dispatch_semaphore_signal(done);
        };
        QZAuthSessionDelegate *delegate;
        if (publicResource) {
            QZResourceSessionDelegate *resource = [QZResourceSessionDelegate new];
            resource.requiresRange = [request valueForHTTPHeaderField:@"Range"].length > 0;
            resource.limit = resource.requiresRange ? 1024 * 1024 : 64 * 1024;
            resource.data = [NSMutableData new];
            resource.completion = completion;
            delegate = resource;
        } else {
            delegate = [QZAuthSessionDelegate new];
        }
        NSURLSession *session = [NSURLSession sessionWithConfiguration:config delegate:delegate delegateQueue:nil];
        NSURLSessionDataTask *task = publicResource ? [session dataTaskWithRequest:request]
            : [session dataTaskWithRequest:request completionHandler:completion];
        [task resume];
        long timedOut = dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, (int64_t)((timeout + 2) * NSEC_PER_SEC)));
        if (timedOut) [task cancel];
        [session invalidateAndCancel];
        [lock lock]; NSDictionary *output = timedOut ? @{@"errorCode":@(-1001)} : result; [lock unlock];
        NSData *json = [NSJSONSerialization dataWithJSONObject:output ?: @{@"errorCode":@(-1011)} options:0 error:nil];
        if (!json) return strdup("{\"errorCode\":-1011}");
        char *buffer = malloc(json.length + 1);
        if (!buffer) return NULL;
        memcpy(buffer, json.bytes, json.length); buffer[json.length] = 0;
        return buffer;
    }
}
