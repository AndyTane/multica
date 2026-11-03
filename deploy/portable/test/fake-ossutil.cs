using System;
using System.IO;
using System.Linq;

// Local transport double only. Never contacts OSS or uses real credentials.
class FakeOssutil {
    static string Option(string[] args, string name) {
        for (int i = 0; i < args.Length; i++) {
            if (args[i] == name && i + 1 < args.Length) return args[i + 1];
            if (args[i].StartsWith(name + "=")) return args[i].Substring(name.Length + 1);
        }
        return "";
    }
    static int Main(string[] args) {
        if (args.Length == 1 && args[0] == "version") { Console.WriteLine("2.4.1"); return 0; }
        try {
            string root = Environment.GetEnvironmentVariable("FAKE_STORE");
            string key = "", operation = "";
            bool put = args.Length > 1 && args[0] == "api" && args[1] == "put-object";
            bool get = args.Length > 2 && args[0] == "cp";
            if (put) { key = Option(args, "--key"); operation = "put"; }
            else if (get) { key = new Uri(args[1]).AbsolutePath.TrimStart('/'); operation = "get"; }
            else throw new Exception("Only direct PUT or exact object download allowed");
            if (args.Any(a => a.StartsWith("--access-key"))) throw new Exception("Credential argument");
            Directory.CreateDirectory(root);
            File.AppendAllText(Path.Combine(root, "calls.txt"), operation + " " + key + " " + Option(args, "--forbid-overwrite") + Environment.NewLine);
            if (key == Environment.GetEnvironmentVariable("FAKE_FAIL_KEY")) {
                Console.WriteLine("LTAI_FAKE_PRIVATE_ID signature=FAKE_SIGNATURE account=123456789");
                Console.Error.WriteLine("FAKE_SECRET_DO_NOT_LOG");
                if (get) File.WriteAllText(args[2], "interrupted");
                return 19;
            }
            string objectPath = Path.Combine(root, key.Replace('/', Path.DirectorySeparatorChar));
            if (put) {
                if (File.Exists(objectPath) && Option(args, "--forbid-overwrite") != "false") return 17;
                Directory.CreateDirectory(Path.GetDirectoryName(objectPath));
                File.Copy(Option(args, "--body").Substring(7), objectPath, true);
            } else {
                if (key == Environment.GetEnvironmentVariable("FAKE_CORRUPT_KEY")) File.WriteAllText(args[2], "corrupted");
                else File.Copy(objectPath, args[2], false);
            }
            return 0;
        } catch { return 23; }
    }
}
