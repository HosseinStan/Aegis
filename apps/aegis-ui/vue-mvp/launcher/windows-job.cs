using System;
using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Text;
using System.Threading;
using Microsoft.Win32.SafeHandles;

namespace AegisLocal {
    // Lifecycle containment only, not a security sandbox. No PID/name-based tree killing.
    public static class WindowsJob {
        [StructLayout(LayoutKind.Sequential)] struct BasicLimits {
            public long ProcessTime, JobTime;
            public uint Flags;
            public UIntPtr MinWorkingSet, MaxWorkingSet;
            public uint ActiveProcessLimit;
            public UIntPtr Affinity;
            public uint Priority, Scheduling;
        }
        [StructLayout(LayoutKind.Sequential)] struct Counters {
            public ulong ReadOps, WriteOps, OtherOps, ReadBytes, WriteBytes, OtherBytes;
        }
        [StructLayout(LayoutKind.Sequential)] struct ExtendedLimits {
            public BasicLimits Basic;
            public Counters IO;
            public UIntPtr ProcessMemory, JobMemory, PeakProcessMemory, PeakJobMemory;
        }
        [DllImport("kernel32.dll", CharSet=CharSet.Unicode, SetLastError=true)]
        static extern SafeFileHandle CreateJobObject(IntPtr attributes, string name);
        [DllImport("kernel32.dll", SetLastError=true)]
        static extern bool SetInformationJobObject(SafeFileHandle job, int infoClass, ref ExtendedLimits limits, uint size);
        [DllImport("kernel32.dll", SetLastError=true)]
        static extern bool AssignProcessToJobObject(SafeFileHandle job, IntPtr process);
        [DllImport("kernel32.dll", SetLastError=true)]
        static extern bool TerminateJobObject(SafeFileHandle job, uint code);
        [DllImport("kernel32.dll", SetLastError=true)]
        static extern bool TerminateProcess(IntPtr process, uint code);
        static Exception WinError(string operation) {
            return new InvalidOperationException("AEGIS_LOCAL_JOB_FAILED: " + operation + " (Windows error " + Marshal.GetLastWin32Error() + "). No containment fallback is allowed.");
        }
        public static int Run(string node, string script, string bootstrap) {
            using (var job=CreateJobObject(IntPtr.Zero,null)) {
                if(job.IsInvalid) throw WinError("CreateJobObject");
                var limits=new ExtendedLimits();
                limits.Basic.Flags=0x2000; // JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE; no breakaway flags.
                if(!SetInformationJobObject(job,9,ref limits,(uint)Marshal.SizeOf<ExtendedLimits>())) throw WinError("SetInformationJobObject");
                using (var process=new Process()) {
                    process.StartInfo=new ProcessStartInfo(node) {
                        UseShellExecute=false, CreateNoWindow=true,
                        WorkingDirectory=System.IO.Path.GetDirectoryName(script),
                        RedirectStandardInput=true, RedirectStandardOutput=true, RedirectStandardError=true,
                        StandardOutputEncoding=Encoding.UTF8, StandardErrorEncoding=Encoding.UTF8
                    };
                    process.StartInfo.ArgumentList.Add(script);
                    // No interpreter preload or inherited frontend secret before the bootstrap gate.
                    process.StartInfo.Environment.Remove("NODE_OPTIONS");
                    process.StartInfo.Environment.Remove("NODE_PATH");
                    foreach(var key in new System.Collections.Generic.List<string>(process.StartInfo.Environment.Keys)) {
                        if(key.StartsWith("VITE_",StringComparison.OrdinalIgnoreCase) || key.StartsWith("AEGIS_",StringComparison.OrdinalIgnoreCase)) process.StartInfo.Environment.Remove(key);
                    }
                    process.OutputDataReceived+=(sender,args)=> { if(args.Data!=null) Console.WriteLine(args.Data); };
                    process.ErrorDataReceived+=(sender,args)=> { if(args.Data!=null) Console.Error.WriteLine(args.Data); };
                    int cancelled=0;
                    ConsoleCancelEventHandler cancel=(sender,args)=> { args.Cancel=true; Interlocked.Exchange(ref cancelled,1); };
                    Console.CancelKeyPress+=cancel;
                    try {
                        if(!process.Start()) throw new InvalidOperationException("AEGIS_LOCAL_START_FAILED: Cannot start the existing Node installation.");
                        if(!AssignProcessToJobObject(job,process.Handle)) {
                            var error=WinError("AssignProcessToJobObject");
                            TerminateProcess(process.Handle,1); // Exact handle of our unbootstrapped child only.
                            throw error;
                        }
                        process.BeginOutputReadLine(); process.BeginErrorReadLine();
                        process.StandardInput.WriteLine(bootstrap); process.StandardInput.Flush();
                        Stopwatch stopping=null;
                        while(!process.WaitForExit(100)) {
                            if(!Console.IsInputRedirected && Console.KeyAvailable) {
                                var key=Console.ReadKey(true);
                                if(key.Key==ConsoleKey.Q) Interlocked.Exchange(ref cancelled,1);
                            }
                            if(Volatile.Read(ref cancelled)!=0 && stopping==null) {
                                stopping=Stopwatch.StartNew();
                                try { process.StandardInput.WriteLine("stop"); process.StandardInput.Flush(); } catch(System.IO.IOException) {}
                            }
                            if(stopping!=null && stopping.Elapsed.TotalSeconds>20) {
                                Console.Error.WriteLine("AEGIS_LOCAL_STOP_TIMEOUT: Terminating only this launcher's Windows Job.");
                                TerminateJobObject(job,130); process.WaitForExit(); return 130;
                            }
                        }
                        process.WaitForExit(); // Drain asynchronous stdout/stderr.
                        return process.ExitCode;
                    } finally {
                        Console.CancelKeyPress-=cancel;
                        // Also catches startup exceptions. Closing the last Job handle kills only its descendants.
                        TerminateJobObject(job,1);
                    }
                }
            }
        }
    }
}
