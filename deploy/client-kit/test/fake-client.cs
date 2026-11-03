using System;
using System.IO;
using System.Linq;

// Isolated lifecycle double; never starts agents or contacts a server.
class FakeClient {
    static int Main(string[] args) {
        string root=Environment.GetEnvironmentVariable("KIT_TEST_ROOT");
        if (String.IsNullOrEmpty(root)) return 90;
        foreach (string key in Environment.GetEnvironmentVariables().Keys) {
            if (key.StartsWith("MULTICA_") || key.StartsWith("U_WORKERS_")) { File.AppendAllText(Path.Combine(root,"calls.txt"),"unexpected-env:"+key+Environment.NewLine); return 91; }
        }
        File.AppendAllText(Path.Combine(root,"calls.txt"),String.Join("|",args)+Environment.NewLine);
        if (args.Length>1 && args[0]=="--profile") args=args.Skip(2).ToArray();
        string state=Path.Combine(root,"state.txt");
        if (args.Length<2) return 92;
        if (args[0]=="daemon" && args[1]=="status") {
            if (Environment.GetEnvironmentVariable("KIT_BAD_JSON")=="1") { Console.WriteLine("invalid"); return 0; }
            if (Environment.GetEnvironmentVariable("KIT_STATUS_FAIL")=="1") return 5;
            string status=File.Exists(state)?File.ReadAllText(state):"stopped";
            Console.WriteLine("{\"status\":\""+status+"\",\"pid\":1234,\"agents\":[\"fixture\"]}");
            return 0;
        }
        if (args[0]=="daemon" && args[1]=="start") {
            if (Environment.GetEnvironmentVariable("KIT_START_FAIL")=="1") return 4;
            File.WriteAllText(state,"running"); return 0;
        }
        if (args[0]=="daemon" && args[1]=="stop") { File.WriteAllText(state,"stopped"); return 0; }
        if (args[0]=="connect") { File.WriteAllText(state,"running"); return 0; }
        if (args[0]=="workspace" && args[1]=="list") {
            if (Environment.GetEnvironmentVariable("KIT_SERVER_FAIL")=="1") return 6;
            Console.WriteLine("[]"); return 0;
        }
        return 93;
    }
}
