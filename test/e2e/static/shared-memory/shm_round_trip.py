# Moves 128MiB through /dev/shm between a child process and its parent with the standard library,
# which fails on the 64MiB container default and passes with the operator's volume.
from multiprocessing import Process, shared_memory

SIZE = 128 * 1024 * 1024
CHUNK = 1024 * 1024


def writer(name):
    shm = shared_memory.SharedMemory(name=name)
    for off in range(0, SIZE, CHUNK):
        shm.buf[off:off + CHUNK] = b"\x5a" * CHUNK
    shm.close()


shm = shared_memory.SharedMemory(create=True, size=SIZE)
try:
    p = Process(target=writer, args=(shm.name,))
    p.start()
    p.join()
    assert p.exitcode == 0, p.exitcode
    assert bytes(shm.buf[:1]) == b"\x5a" and bytes(shm.buf[SIZE - 1:SIZE]) == b"\x5a"
finally:
    shm.close()
    shm.unlink()
print("round trip ok")
