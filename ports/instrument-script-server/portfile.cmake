vcpkg_from_github(
    OUT_SOURCE_PATH SOURCE_PATH
    REPO falcon-autotuning/instrument-script-server
    REF v${VERSION}
    SHA512 b303f1ab4d7051b7df81c5f7f80328fd68a457ea17236ea1ed616fd69738dc10470d2d11a5461a3fd798e50de9eda42ac71f40f703aff34b114d24ff6b8f2ac7
)

# Local workspace override for shutdown-order stability fix in ProcessManager singleton.
# Keep the overlay port self-contained by injecting the patched source file.
get_filename_component(WORKSPACE_ROOT "${CURRENT_PORT_DIR}/../../.." ABSOLUTE)
set(ISS_PROXY_SRC_OVERRIDE "${WORKSPACE_ROOT}/instrument-script-server/src/server/InstrumentWorkerProxy.cpp")
if(EXISTS "${ISS_PROXY_SRC_OVERRIDE}")
    file(COPY "${ISS_PROXY_SRC_OVERRIDE}" DESTINATION "${SOURCE_PATH}/src/server")
endif()

vcpkg_cmake_configure(
    SOURCE_PATH "${SOURCE_PATH}"
)

vcpkg_cmake_install()
vcpkg_cmake_config_fixup()

file(INSTALL "${SOURCE_PATH}/LICENSE"
     DESTINATION "${CURRENT_PACKAGES_DIR}/share/${PORT}"
     RENAME copyright)

vcpkg_copy_pdbs()
