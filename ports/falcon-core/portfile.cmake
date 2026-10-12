vcpkg_from_github(
    OUT_SOURCE_PATH SOURCE_PATH
    REPO falcon-autotuning/falcon-core
    REF v${VERSION}
    SHA512 332466ae1b3f75217f2ae8abdf31209b9959b917da5a80006da5d0719b0de2b14c71a012aa39a0795315909372c3c303663ba2721bcf5b65253db573133a99da
)

set(BUILD_C_API OFF)
if("c-api" IN_LIST FEATURES)
  set(BUILD_C_API ON)
endif()

vcpkg_cmake_configure(
    SOURCE_PATH "${SOURCE_PATH}"
    OPTIONS
        -DFALCON_CORE_BUILD_C_API=${BUILD_C_API}
)

vcpkg_cmake_install()
vcpkg_cmake_config_fixup()

file(INSTALL "${SOURCE_PATH}/LICENSE.txt"
     DESTINATION "${CURRENT_PACKAGES_DIR}/share/${PORT}"
     RENAME copyright)

vcpkg_copy_pdbs()
