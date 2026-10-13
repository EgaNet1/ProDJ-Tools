// The Explorer-style folder dialog (IFileOpenDialog with FOS_PICKFOLDERS).
// Interfaces list their methods in vtable order; methods that are never
// called are declared without parameters only to keep the slots aligned.
using System;
using System.Runtime.InteropServices;

namespace ProDJ {
    public static class FolderPicker {
        [ComImport, Guid("DC1C5A9C-E88A-4dde-A5A1-60F82A20AEF7")]
        class FileOpenDialog {}

        [ComImport, Guid("42f85136-db7e-439c-85f1-e4075d135fc8"), InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
        interface IFileDialog {
            [PreserveSig] int Show(IntPtr owner);
            void SetFileTypes();
            void SetFileTypeIndex();
            void GetFileTypeIndex();
            void Advise();
            void Unadvise();
            void SetOptions(uint options);
            void GetOptions();
            void SetDefaultFolder();
            void SetFolder(IShellItem item);
            void GetFolder();
            void GetCurrentSelection();
            void SetFileName();
            void GetFileName();
            void SetTitle([MarshalAs(UnmanagedType.LPWStr)] string title);
            void SetOkButtonLabel();
            void SetFileNameLabel();
            void GetResult(out IShellItem item);
        }

        [ComImport, Guid("43826D1E-E718-42EE-BC55-A1E261C37BFE"), InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
        interface IShellItem {
            void BindToHandler();
            void GetParent();
            void GetDisplayName(uint type, [MarshalAs(UnmanagedType.LPWStr)] out string name);
        }

        [DllImport("shell32.dll", CharSet = CharSet.Unicode, PreserveSig = false)]
        static extern void SHCreateItemFromParsingName(
            string path, IntPtr bindContext, [MarshalAs(UnmanagedType.LPStruct)] Guid iid, out IShellItem item);

        [DllImport("user32.dll")]
        static extern IntPtr GetForegroundWindow();

        const uint PickFolders = 0x20, ForceFileSystem = 0x40;
        const uint FileSystemPath = 0x80058000;

        public static string Pick(string title, string start) {
            var dialog = (IFileDialog)new FileOpenDialog();
            dialog.SetOptions(PickFolders | ForceFileSystem);
            dialog.SetTitle(title);
            if (!string.IsNullOrEmpty(start)) {
                try {
                    IShellItem folder;
                    SHCreateItemFromParsingName(start, IntPtr.Zero, typeof(IShellItem).GUID, out folder);
                    dialog.SetFolder(folder);
                } catch (Exception) {
                    // A missing start folder is not worth failing over.
                }
            }
            if (dialog.Show(GetForegroundWindow()) != 0) {
                return "";
            }
            IShellItem result;
            dialog.GetResult(out result);
            string path;
            result.GetDisplayName(FileSystemPath, out path);
            return path;
        }
    }
}
