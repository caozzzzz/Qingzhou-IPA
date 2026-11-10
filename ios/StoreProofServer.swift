import Foundation
import Network

// Loopback-only runtime/provisioning transport. No account credentials enter it.
final class StoreProofServer: @unchecked Sendable {
    private let queue = DispatchQueue(label: "Qingzhou.proof.loopback")
    private var listener: NWListener?
    private let token = UUID().uuidString
    private(set) var baseURL: URL?
    private let files = Set(["index.html","sap-signer.js","sap.wasm","wasm_exec.js","unicorn_x86.js","libCoreADI.so","libstoreservicescore.so"])
    func start() async throws -> URL {
        if let baseURL {return baseURL}
        return try await withCheckedThrowingContinuation { continuation in
            do {
                let parameters = NWParameters.tcp
                parameters.requiredLocalEndpoint = .hostPort(host: "127.0.0.1",port: .any)
                let server=try NWListener(using:parameters)
                listener=server
                var completed=false
                server.stateUpdateHandler = { [weak self] state in
                    guard !completed else {return}
                    switch state {
                    case .ready:
                        guard let self, let port=server.port else {return}
                        completed=true
                        let url=URL(string:"http://127.0.0.1:\(port.rawValue)/\(self.token)/")!
                        self.baseURL=url;continuation.resume(returning:url)
                    case .failed, .cancelled: completed=true;continuation.resume(throwing:ProofFailure.unavailable)
                    default: break
                    }
                }
                server.newConnectionHandler = { [weak self] connection in
                    connection.start(queue:self?.queue ?? .global())
                    self?.read(connection,Data())
                    self?.queue.asyncAfter(deadline:.now()+60) {connection.cancel()}
                }
                server.start(queue:queue)
            } catch {continuation.resume(throwing:ProofFailure.unavailable)}
        }
    }
    func stop() {listener?.cancel();listener=nil;baseURL=nil}
    private func read(_ connection:NWConnection,_ previous:Data) {
        connection.receive(minimumIncompleteLength:1,maximumLength:65536) { [weak self] data,_,done,error in
            guard let self else {connection.cancel();return}
            var buffer=previous;buffer.append(data ?? Data())
            guard error==nil,buffer.count<=1100000 else {connection.cancel();return}
            if let boundary=buffer.range(of:Data("\r\n\r\n".utf8)),let header=String(data:buffer[..<boundary.lowerBound],encoding:.utf8) {
                let lines=header.components(separatedBy:"\r\n"),request=(lines.first ?? "").split(separator:" ")
                guard request.count==3 else {self.reply(connection,400);return}
                var fields=[String:String]()
                for line in lines.dropFirst() {if let index=line.firstIndex(of:":"){let key=String(line[..<index]).lowercased();guard fields[key]==nil else {self.reply(connection,400);return};fields[key]=line[line.index(after:index)...].trimmingCharacters(in:.whitespaces)}}
                guard fields["transfer-encoding"]==nil,let length=Int(fields["content-length"] ?? "0"),length>=0,length<=1048576 else {self.reply(connection,400);return}
                let start=boundary.upperBound
                if buffer.count-start>=length {
                    self.route(connection,String(request[0]),String(request[1]),Data(buffer[start..<(start+length)]));return
                }
            }
            if done {connection.cancel()} else {self.read(connection,buffer)}
        }
    }
    private func route(_ connection:NWConnection,_ method:String,_ target:String,_ body:Data) {
        guard let url=URLComponents(string:"http://localhost"+target),url.path.hasPrefix("/"+token+"/") else {reply(connection,403);return}
        let path=String(url.path.dropFirst(token.count+2))
        if method=="OPTIONS" {reply(connection,204);return}
        if path.hasPrefix("runtime/"),method=="GET" {
            let name=String(path.dropFirst(8))
            guard files.contains(name),let root=Bundle.main.url(forResource:"StoreRuntime",withExtension:nil),let data=try? Data(contentsOf:root.appendingPathComponent(name)) else {reply(connection,404);return}
            let type=name.hasSuffix(".js") ? "application/javascript" : name.hasSuffix(".wasm") ? "application/wasm" : name.hasSuffix(".html") ? "text/html" : "application/octet-stream"
            reply(connection,200,data,type);return
        }
        guard path=="proxy",method=="POST",url.queryItems?.count==1,
              url.queryItems?.first?.name=="url",url.queryItems?.first?.value=="https://auth.itunes.apple.com/auth/v1/otp/1.5/finish" else {reply(connection,403);return}
        // Validate the finish body before sending. This route accepts only device
        // setup data, never Apple ID/password/cookie fields.
        guard let raw=try? ProofPropertyList.dictionary(body),Set(raw.keys)==Set(["clientData","guid"]),let guid=raw["guid"] as? String,guid.range(of:"^[0-9A-F]{12}$",options:.regularExpression) != nil,let client=raw["clientData"] as? String,let decoded=Data(base64Encoded:client),!decoded.isEmpty else {reply(connection,400);return}
        Task {
            do {
                var request=URLRequest(url:URL(string:"https://auth.itunes.apple.com/auth/v1/otp/1.5/finish")!)
                request.httpMethod="POST";request.httpBody=body;request.timeoutInterval=45
                request.setValue("application/x-apple-plist",forHTTPHeaderField:"Content-Type")
                request.setValue("Configurator/2.21 (Macintosh; OS X 15.2; 24C5089c) AppleWebKit/0620.1.16.11.6",forHTTPHeaderField:"User-Agent")
                let config=URLSessionConfiguration.ephemeral;config.httpCookieStorage=nil;config.httpShouldSetCookies=false;config.urlCredentialStorage=nil
                let session=URLSession(configuration:config,delegate:ProofNoRedirect(),delegateQueue:nil);defer{session.invalidateAndCancel()}
                let (data,response)=try await session.data(for:request)
                guard let response=response as? HTTPURLResponse,response.statusCode==200,data.count<=1048576 else {self.reply(connection,502);return}
                let values=try ProofPropertyList.dictionary(data)
                let normalized=try PropertyListSerialization.data(fromPropertyList:values,format:.xml,options:0)
                self.reply(connection,200,normalized,"application/x-apple-plist")
            } catch {self.reply(connection,502)}
        }
    }
    private func reply(_ connection:NWConnection,_ status:Int,_ body:Data=Data(),_ type:String="text/plain") {
        let header="HTTP/1.1 \(status) Response\r\nContent-Length: \(body.count)\r\nContent-Type: \(type)\r\nCache-Control: no-store\r\nConnection: close\r\n\r\n"
        var data=Data(header.utf8);data.append(body)
        connection.send(content:data,completion:.contentProcessed{_ in connection.cancel()})
    }
}
final class ProofNoRedirect:NSObject,URLSessionTaskDelegate {
    func urlSession(_ session:URLSession,task:URLSessionTask,willPerformHTTPRedirection response:HTTPURLResponse,newRequest request:URLRequest,completionHandler:@escaping(URLRequest?)->Void){completionHandler(nil)}
}
enum ProofFailure:Error {case unavailable}
