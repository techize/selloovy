# Go lesson 12: bounded images and immutable media

An upload is untrusted input. photoimage.Decode reads dimensions with
image.DecodeConfig before allocating pixels, restricts format/size, then decodes
and re-encodes the image. Byte limits alone would still allow a small compressed
file to request a huge pixel allocation.

A buffered channel acts as a small processing semaphore. Sending acquires a slot,
and a deferred receive releases it on every return path. At most two conversions
run in this process; a saturated request can retry after the service responds.

JPEG orientation is read using bounded byte slices and encoding/binary. The pixel
transforms run before EXIF is removed. Re-encoding retains pixels rather than
trusting the original container or its metadata.

SavePhoto first verifies the owned product, processes the image, then locks and
revalidates the owner/session and shared revision in a transaction. Slow decoding
does not hold a database write lock. A second save can win during decoding; the
revision check makes the first stale rather than overwriting the winner.

An immutable photo ID lets the draft selection and publication reference differ.
Cleanup deletes only content referenced by neither. Changing a draft cannot
silently replace pixels already reviewed for publication.

Optional exercise: trace replacing an already published photo, then removing the
draft replacement and republishing. Explain which photo references exist at each
step, and why returning public bytes by photo ID alone would be unsafe.
